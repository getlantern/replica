package replica

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
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

// Storage defines the common API for the cloud object storage
type Storage interface {
	Get(endpoint Endpoint, key string) (io.ReadCloser, error)
	Put(endpoint Endpoint, key string, r io.Reader) error
	Delete(endpoint Endpoint, key string) error
}

type Client struct {
	Storage  Storage
	Endpoint Endpoint
}

func (client *Client) GetObject(key string) (io.ReadCloser, error) {
	return client.Storage.Get(client.Endpoint, key)
}

// GetMetainfo retrieves the metainfo object for the given prefix from S3.
func (client *Client) GetMetainfo(s3Prefix Upload) (io.ReadCloser, error) {
	return client.Storage.Get(s3Prefix.Endpoint, s3Prefix.TorrentKey())
}

// Upload creates a new Replica object from the Reader with the given name. Returns the replica magnet link
func (client *Client) Upload(read io.Reader, uConfig UploadConfig) (result UploadMetainfo, err error) {
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
	result.Upload = client.Endpoint.NewUpload(uConfig)
	err = client.Storage.Put(client.Endpoint, result.FileDataKey(uConfig.Filename()), read)
	// Synchronize with the piece generation.
	piecesWriter.CloseWithError(err)
	<-piecesDone
	if err != nil {
		err = fmt.Errorf("uploading to %s: %w", client.Endpoint.StorageProvider, err)
		return
	}
	if piecesErr != nil {
		err = fmt.Errorf("generating metainfo pieces: %w", piecesErr)
		return
	}

	result.Info = metainfo.Info{
		PieceLength: pieceLength,
		Name:        result.Upload.String(),
		Pieces:      pieces,
		Files: []metainfo.FileInfo{
			{Length: cw.BytesWritten, Path: []string{uConfig.Filename()}},
		},
	}
	infoBytes, err := bencode.Marshal(result.Info)
	if err != nil {
		panic(err)
	}
	result.MetaInfo = &metainfo.MetaInfo{
		InfoBytes:    infoBytes,
		CreationDate: time.Now().Unix(),
		Comment:      result.Upload.ExactSource(),
		UrlList:      result.Upload.WebseedUrls(),
	}
	r, w := io.Pipe()
	go func() {
		err := result.MetaInfo.Write(w)
		w.CloseWithError(err)
	}()
	err = client.Storage.Put(result.Upload.Endpoint, result.Upload.TorrentKey(), r)
	if err != nil {
		err = fmt.Errorf("uploading metainfo: %w", err)
		return
	}
	return
}

// UploadFile uploads the file for the given name, returning the Replica magnet link for the upload.
func (client *Client) UploadFile(uConfig UploadConfig) (UploadMetainfo, error) {
	f, err := os.Open(uConfig.FullPath())
	if err != nil {
		return UploadMetainfo{}, fmt.Errorf("opening file %q: %w", uConfig.FullPath(), err)
	}
	defer f.Close()
	return client.Upload(f, uConfig)
}

// Deletes the S3 file with the given key.
func (client *Client) DeleteUpload(upload Upload, files ...[]string) (errs []error) {
	delete := func(key string) {
		if err := client.Storage.Delete(upload.Endpoint, key); err != nil {
			errs = append(errs, fmt.Errorf("deleting %q: %w", key, err))
		}
	}
	delete(upload.TorrentKey())
	for _, f := range files {
		delete(upload.FileDataKey(path.Join(f...)))
	}
	return errs
}
