package replica

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/anacrolix/envpprof"
	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"golang.org/x/xerrors"
)

// Replica is the interface for files posted to Replica.
type Replica interface {
	UploadFile(filename string) (UploadOutput, error)
	GetTorrent(key string) error
	IterUploads(dir string, f func(IteredUpload)) error
	Upload(ior io.Reader, fileName string) (output UploadOutput, err error)
	DeletePrefix(s3Prefix S3Prefix, files ...[]string) []error
	GetMetainfo(s3Prefix S3Prefix) (io.ReadCloser, error)
}

type replica struct {
	httpClient *http.Client
}

// New creates a new Replica instance with the specified http client.
func New(httpClient *http.Client) Replica {
	return &replica{
		httpClient: httpClient,
	}
}

func (r *replica) newSession() (*session.Session, error) {
	creds, err := creds.getCredentials()
	if err != nil {
		return nil, xerrors.Errorf("could not get creds: %v", err)
	}

	return session.Must(session.NewSession(&aws.Config{
		Credentials: creds,
		Region:      aws.String(region),
		HTTPClient:  r.httpClient,
	})), nil
}

func (r *replica) GetObject(key string) (io.ReadCloser, error) {
	sess, err := r.newSession()
	if err != nil {
		return nil, fmt.Errorf("getting new session: %w", err)
	}
	cl := s3.New(sess)
	out, err := cl.GetObject(&s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("getting s3 object: %w", err)
	}
	return out.Body, nil

}

// GetMetainfo retrieves the metainfo object for the given prefix from S3.
func (r *replica) GetMetainfo(s3Prefix S3Prefix) (io.ReadCloser, error) {
	return r.GetObject(s3Prefix.TorrentKey())
}

type UploadOutput struct {
	S3Prefix S3Prefix
	Metainfo *metainfo.MetaInfo
	Info     Info
}

// Upload creates a new Replica object from the Reader with the given name. Returns the objects S3 UUID
// prefix.
func (r *replica) Upload(read io.Reader, fileName string) (output UploadOutput, err error) {
	sess, err := r.newSession()
	if err != nil {
		err = fmt.Errorf("getting aws session: %w", err)
		return
	}

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
	output.S3Prefix = NewPrefix()
	uploader := s3manager.NewUploader(sess)
	_, err = uploader.Upload(&s3manager.UploadInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(output.S3Prefix.FileDataKey(fileName)),
		Body:   read,
	})
	// Synchronize with the piece generation.
	piecesWriter.CloseWithError(err)
	<-piecesDone
	if err != nil {
		err = fmt.Errorf("uploading to s3: %w", err)
		return
	}
	if piecesErr != nil {
		err = fmt.Errorf("generating metainfo pieces: %w", piecesErr)
		return
	}

	output.Info.TorrentInfo = &metainfo.Info{
		PieceLength: pieceLength,
		Name:        output.S3Prefix.String(),
		Pieces:      pieces,
		Files: []metainfo.FileInfo{
			{Length: cw.BytesWritten, Path: []string{fileName}},
		},
	}
	infoBytes, err := bencode.Marshal(output.Info.TorrentInfo)
	if err != nil {
		panic(err)
	}
	output.Metainfo = &metainfo.MetaInfo{
		InfoBytes:    infoBytes,
		CreationDate: time.Now().Unix(),
		Comment:      "Replica",
		UrlList:      []string{output.S3Prefix.WebseedUrl()},
	}
	err = uploadMetainfo(output.S3Prefix, output.Metainfo, uploader)
	if err != nil {
		err = fmt.Errorf("uploading metainfo: %w", err)
		return
	}
	return
}

func uploadMetainfo(prefix S3Prefix, mi *metainfo.MetaInfo, uploader *s3manager.Uploader) error {
	r, w := io.Pipe()
	go func() {
		err := mi.Write(w)
		w.CloseWithError(err)
	}()
	_, err := uploader.Upload(&s3manager.UploadInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(prefix.TorrentKey()),
		Body:   r,
	})
	return err
}

// UploadFile uploads the file for the given name, returning the Replica UUID prefix for the upload.
func (r *replica) UploadFile(filename string) (UploadOutput, error) {
	f, err := os.Open(filename)
	if err != nil {
		return UploadOutput{}, fmt.Errorf("opening file %q: %w", filename, err)
	}
	defer f.Close()
	return r.Upload(f, filepath.Base(filename))
}

// DeletePrefix deletes the S3 file with the given key.
func (r *replica) DeletePrefix(s3Prefix S3Prefix, files ...[]string) []error {
	sess, err := r.newSession()
	if err != nil {
		return []error{fmt.Errorf("getting new session: %w", err)}
	}
	svc := s3.New(sess)
	var errs []error
	delete := func(key string) {
		input := &s3.DeleteObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(key),
		}
		_, err := svc.DeleteObject(input)
		if err != nil {
			errs = append(errs, fmt.Errorf("deleting %q: %w", key, err))
		}
	}
	delete(s3Prefix.TorrentKey())
	for _, f := range files {
		delete(s3Prefix.FileDataKey(path.Join(f...)))
	}
	return errs
}

// GetObjectTorrent returns the object metainfo for the given key.
func (r *replica) GetObjectTorrent(key string) (io.ReadCloser, error) {
	sess, err := r.newSession()
	if err != nil {
		return nil, xerrors.Errorf("Could not get session: %v", err)
	}
	svc := s3.New(sess)
	out, err := svc.GetObjectTorrent(&s3.GetObjectTorrentInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, err
	}
	return out.Body, nil
}

// GetTorrent downloads the metainfo for the Replica object to a .torrent file in the current working directory.
func (r *replica) GetTorrent(key string) error {
	t, err := r.GetObjectTorrent(key)
	if err != nil {
		return xerrors.Errorf("getting object torrent: %w", err)
	}
	defer t.Close()
	f, err := os.OpenFile(path.Base(key)+".torrent", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0640)
	if err != nil {
		return xerrors.Errorf("opening output file: %w", err)
	}
	defer f.Close()
	if _, err := io.Copy(f, t); err != nil {
		return xerrors.Errorf("copying torrent: %w", err)
	}
	if err := f.Close(); err != nil {
		return xerrors.Errorf("closing torrent file: %w", f.Close())
	}
	return nil
}

type IteredUpload struct {
	Metainfo *metainfo.MetaInfo
	FileInfo os.FileInfo
	Err      error
}

func (me *IteredUpload) S3Prefix() S3Prefix {
	return S3Prefix(strings.TrimSuffix(me.FileInfo.Name(), filepath.Ext(me.FileInfo.Name())))
}

// IterUploads walks the torrent files stored in the directory.
func (r *replica) IterUploads(dir string, f func(IteredUpload)) error {
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
		f(IteredUpload{Metainfo: mi, FileInfo: e})
	}
	return nil
}
