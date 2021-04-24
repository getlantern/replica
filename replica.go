package replica

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"time"

	_ "github.com/anacrolix/envpprof"
	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/metainfo"
)

func CreateLink(ih torrent.InfoHash, s3upload Upload, filePath []string) string {
	return metainfo.Magnet{
		InfoHash:    ih,
		DisplayName: path.Join(filePath...),
		Params: url.Values{
			"as": s3upload.MetainfoUrls(),
			"xs": {s3upload.ExactSource()},
			// This might technically be more correct, but I couldn't find any torrent client that
			// supports it. Make sure to change any assumptions about "xs" before changing it.
			//"xs": {fmt.Sprintf("https://getlantern-replica.s3-ap-southeast-1.amazonaws.com/%s/torrent", s3upload)},

			// Since S3 key is provided, we know that it must be a single-file torrent.
			"so": {"0"},
			"ws": s3upload.WebseedUrls(),
		},
	}.String()
}

// Storage defines the common API for the cloud object storage.
type StorageClient interface {
	Get(key string) (io.ReadCloser, error)
	Put(key string, r io.Reader) error
	Delete(key string) error
	// Could we return an Endpoint too? Since all endpoints should be able to be used to construct
	// StorageClients.
}

type Client struct {
	StorageClient
	Endpoint
	ServiceClient
}

type ServiceClient struct {
	// This should be a URL to handle uploads. The specifics are in replica-rust. It's not clear how
	// this might relate to other operations that would use Endpoint at present. Uploading might be
	// distinct from other client operations now.
	ReplicaServiceEndpoint *url.URL
	HttpClient             *http.Client
}

type UploadOutput struct {
	UploadMetainfo
	AuthToken *string
	Link      *string
}

func (cl ServiceClient) Upload(read io.Reader, fileName string) (output UploadOutput, err error) {
	req, err := http.NewRequest(http.MethodPut, serviceUploadUrl(cl.ReplicaServiceEndpoint, fileName).String(), read)
	if err != nil {
		err = fmt.Errorf("creating put request: %w", err)
		return
	}
	resp, err := cl.HttpClient.Do(req)
	if err != nil {
		err = fmt.Errorf("doing request: %w", err)
		return
	}
	defer resp.Body.Close()
	respBodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		err = fmt.Errorf("reading all response body bytes: %w", err)
		return
	}
	var serviceOutput ServiceUploadOutput
	err = json.Unmarshal(respBodyBytes, &serviceOutput)
	if err != nil {
		err = fmt.Errorf("decoding response %q: %w", string(respBodyBytes), err)
		return
	}
	output.Link = &serviceOutput.Link
	output.AuthToken = &serviceOutput.AdminToken
	var metainfoBytesBuffer bytes.Buffer
	for _, r := range serviceOutput.Metainfo {
		if r < 0 || r > math.MaxUint8 {
			err = fmt.Errorf("response metainfo rune has unexpected codepoint")
			return
		}
		err = metainfoBytesBuffer.WriteByte(byte(r))
		if err != nil {
			panic(err)
		}
	}
	mi, err := metainfo.Load(&metainfoBytesBuffer)
	if err != nil {
		err = fmt.Errorf("parsing metainfo from response: %w", err)
		return
	}
	output.MetaInfo = mi
	output.Info, err = mi.UnmarshalInfo()
	if err != nil {
		err = fmt.Errorf("unmarshalling info from response metainfo bytes: %w", err)
		return
	}
	m, err := metainfo.ParseMagnetURI(serviceOutput.Link)
	if err != nil {
		err = fmt.Errorf("parsing response replica link: %w", err)
		return
	}
	err = output.Upload.FromMagnet(m)
	if err != nil {
		err = fmt.Errorf("extracting upload specifics from response replica link: %w", err)
		return
	}
	return
}

// Deprecated: Uploads directly to storage in Go are now unsupported. Use an intermediary service
// like replica-rust for this. Uploading with custom endpoints/non-UUID prefixes may be an exception
// for now.
//
// Upload creates a new Replica object from the Reader with the given name. Returns the replica
// magnet link.
func (cl Client) UploadDirectly(read io.Reader, uConfig UploadConfig) (output UploadOutput, err error) {
	piecesReader, piecesWriter := io.Pipe()
	read = io.TeeReader(read, piecesWriter)

	var cw CountWriter
	read = io.TeeReader(read, &cw)
	// 256 KiB is what s3 would use. We want to balance chunks per piece, metainfo size, and having
	// too many pieces. This can be changed any time, since it only affects future metainfos.
	const pieceLength = 1 << 18
	var (
		pieces     []byte
		piecesErr  error
		piecesDone = make(chan struct{})
	)
	go func() {
		defer close(piecesDone)
		pieces, piecesErr = metainfo.GeneratePieces(piecesReader, pieceLength, nil)
	}()

	// Whether we fail or not from this point, the prefix could be useful to the caller.
	output.Upload = NewUpload(uConfig, cl.Endpoint)
	err = cl.Put(output.FileDataKey(uConfig.Filename()), read)
	// Synchronize with the piece generation.
	piecesWriter.CloseWithError(err)
	<-piecesDone
	if err != nil {
		err = fmt.Errorf("uploading to %v: %w", cl.Endpoint, err)
		return
	}
	if piecesErr != nil {
		err = fmt.Errorf("generating metainfo pieces: %w", piecesErr)
		return
	}

	output.Info = metainfo.Info{
		PieceLength: pieceLength,
		Name:        output.Upload.String(),
		Pieces:      pieces,
		Files: []metainfo.FileInfo{
			{Length: cw.BytesWritten, Path: []string{uConfig.Filename()}},
		},
	}
	infoBytes, err := bencode.Marshal(output.Info)
	if err != nil {
		panic(err)
	}
	output.MetaInfo = &metainfo.MetaInfo{
		InfoBytes:    infoBytes,
		CreationDate: time.Now().Unix(),
		Comment:      output.Upload.ExactSource(),
		UrlList:      output.Upload.WebseedUrls(),
	}
	r, w := io.Pipe()
	go func() {
		err := output.MetaInfo.Write(w)
		w.CloseWithError(err)
	}()
	err = cl.Put(output.Upload.TorrentKey(), r)
	if err != nil {
		err = fmt.Errorf("uploading metainfo: %w", err)
		return
	}
	return
}

// UploadFile uploads the file for the given name, returning the Replica magnet link for the upload.
func (cl Client) UploadFile(fileName, uploadedAsName string) (_ UploadOutput, err error) {
	f, err := os.Open(fileName)
	if err != nil {
		err = fmt.Errorf("opening file: %w", err)
		return
	}
	defer f.Close()
	return cl.Upload(f, uploadedAsName)
}

// UploadFile uploads the file for the given name, returning the Replica magnet link for the upload.
func (cl Client) UploadFileDirectly(uConfig UploadConfig) (_ UploadOutput, err error) {
	f, err := os.Open(uConfig.FullPath())
	if err != nil {
		err = fmt.Errorf("opening file %q: %w", uConfig.FullPath(), err)
		return
	}
	defer f.Close()
	return cl.UploadDirectly(f, uConfig)
}

func (cl ServiceClient) DeleteUpload(prefix Prefix, auth string, haveMetainfo bool) error {
	data := url.Values{
		"prefix": {prefix.PrefixString()},
		"auth":   {auth},
	}
	if haveMetainfo {
		// We only use this field if we need to, to prevent detection and for backward compatibility.
		data["have_metainfo"] = []string{"true"}
	}
	resp, err := cl.HttpClient.PostForm(
		serviceDeleteUrl(cl.ReplicaServiceEndpoint).String(),
		data,
	)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected response: %q", resp.Status)
	}
	return nil
}

// Deletes the S3 file with the given key. TODO: Delete admin token too.
//
// Deprecated: Delete via replica-rust instead.
func (cl Client) DeleteUploadDirectly(upload Upload, files ...[]string) (errs []error) {
	delete := func(key string) {
		if err := cl.Delete(key); err != nil {
			errs = append(errs, fmt.Errorf("deleting %q: %w", key, err))
		}
	}
	delete(upload.TorrentKey())
	for _, f := range files {
		delete(upload.FileDataKey(path.Join(f...)))
	}
	return errs
}

type IteredUpload struct {
	Metainfo UploadMetainfo
	FileInfo os.FileInfo
	Err      error
}

// IterUploads walks the torrent files (UUID-uploads?) stored in the directory. This is specific to
// the replica desktop server, except that maybe there is replica-project specific stuff to extract
// from metainfos etc.
func IterUploads(dir string, f func(IteredUpload)) error {
	entries, err := ioutil.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, e := range entries {
		p := filepath.Join(dir, e.Name())
		mi, err := metainfo.LoadFromFile(p)
		if err != nil {
			f(IteredUpload{Err: fmt.Errorf("loading metainfo from file %q: %w", p, err)})
			continue
		}
		var umi UploadMetainfo
		err = umi.FromTorrentMetainfo(mi)
		if err != nil {
			f(IteredUpload{Err: fmt.Errorf("unwrapping upload metainfo from file %q: %w", p, err)})
			continue
		}
		f(IteredUpload{Metainfo: umi, FileInfo: e})
	}
	return nil
}
