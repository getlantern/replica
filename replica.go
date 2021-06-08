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

	_ "github.com/anacrolix/envpprof"
	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
)

func CreateLink(ih torrent.InfoHash, s3upload Prefix, filePath []string) string {
	return metainfo.Magnet{
		InfoHash:    ih,
		DisplayName: path.Join(filePath...),
		Params: url.Values{
			"xs": {ExactSource(s3upload)},
			// Since S3 key is provided, we know that it must be a single-file torrent.
			"so": {"0"},
		},
	}.String()
}

type ServiceClient struct {
	// This should be a URL to handle uploads. The specifics are in replica-rust.
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

// UploadFile uploads the file for the given name, returning the Replica magnet link for the upload.
func (cl ServiceClient) UploadFile(fileName, uploadedAsName string) (_ UploadOutput, err error) {
	f, err := os.Open(fileName)
	if err != nil {
		err = fmt.Errorf("opening file: %w", err)
		return
	}
	defer f.Close()
	return cl.Upload(f, uploadedAsName)
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
