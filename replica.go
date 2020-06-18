package replica

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"path/filepath"
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

type Client struct {
	HttpClient *http.Client
	Endpoint
	creds cognitoProvider
}

var DefaultEndpoint = Endpoint{
	BucketName: "getlantern-replica",
	AwsRegion:  "ap-southeast-1",
}

func (r *Client) newSession() (*session.Session, error) {
	creds, err := r.creds.getCredentials()
	if err != nil {
		return nil, xerrors.Errorf("could not get creds: %v", err)
	}

	return session.Must(session.NewSession(&aws.Config{
		Credentials:      creds,
		Region:           aws.String(r.AwsRegion),
		HTTPClient:       r.HttpClient,
		S3ForcePathStyle: aws.Bool(true),
	})), nil
}

func (r *Client) GetObject(key string) (io.ReadCloser, error) {
	sess, err := r.newSession()
	if err != nil {
		return nil, fmt.Errorf("getting new session: %w", err)
	}
	cl := s3.New(sess)
	out, err := cl.GetObject(&s3.GetObjectInput{
		Bucket: aws.String(r.BucketName),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("getting s3 object: %w", err)
	}
	return out.Body, nil

}

// GetMetainfo retrieves the metainfo object for the given prefix from S3.
func (r *Client) GetMetainfo(s3Prefix Upload) (io.ReadCloser, error) {
	return r.GetObject(s3Prefix.TorrentKey())
}

type UploadOutput struct {
	UploadMetainfo
}

// Upload creates a new Replica object from the Reader with the given name. Returns the objects S3 UUID
// prefix.
func (r *Client) Upload(read io.Reader, fileName string) (output UploadOutput, err error) {
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
	output.Upload = r.NewUpload()
	uploader := s3manager.NewUploader(sess)
	_, err = uploader.Upload(&s3manager.UploadInput{
		Bucket: aws.String(r.BucketName),
		Key:    aws.String(output.FileDataKey(fileName)),
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

	output.Info = metainfo.Info{
		PieceLength: pieceLength,
		Name:        output.Upload.String(),
		Pieces:      pieces,
		Files: []metainfo.FileInfo{
			{Length: cw.BytesWritten, Path: []string{fileName}},
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
	err = uploadMetainfo(output.Upload, output.MetaInfo, uploader)
	if err != nil {
		err = fmt.Errorf("uploading metainfo: %w", err)
		return
	}
	return
}

func uploadMetainfo(prefix Upload, mi *metainfo.MetaInfo, uploader *s3manager.Uploader) error {
	r, w := io.Pipe()
	go func() {
		err := mi.Write(w)
		w.CloseWithError(err)
	}()
	_, err := uploader.Upload(&s3manager.UploadInput{
		Bucket: aws.String(prefix.BucketName),
		Key:    aws.String(prefix.TorrentKey()),
		Body:   r,
	})
	return err
}

// UploadFile uploads the file for the given name, returning the Replica UUID prefix for the upload.
func (r *Client) UploadFile(filename string) (UploadOutput, error) {
	f, err := os.Open(filename)
	if err != nil {
		return UploadOutput{}, fmt.Errorf("opening file %q: %w", filename, err)
	}
	defer f.Close()
	return r.Upload(f, filepath.Base(filename))
}

// Deletes the S3 file with the given key.
func (r *Client) DeleteUpload(upload Upload, files ...[]string) []error {
	sess, err := r.newSession()
	if err != nil {
		return []error{fmt.Errorf("getting new session: %w", err)}
	}
	svc := s3.New(sess)
	var errs []error
	delete := func(key string) {
		input := &s3.DeleteObjectInput{
			Bucket: aws.String(upload.BucketName),
			Key:    aws.String(key),
		}
		_, err := svc.DeleteObject(input)
		if err != nil {
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

// IterUploads walks the torrent files stored in the directory.
func (r *Endpoint) IterUploads(dir string, f func(IteredUpload)) error {
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
