package replica

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"

	_ "github.com/anacrolix/envpprof"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"golang.org/x/xerrors"

	"github.com/google/uuid"
)

func newSession() (*session.Session, error) {
	creds, err := creds.getCredentials()
	if err != nil {
		return nil, xerrors.Errorf("could not get creds: %v", err)
	}

	return session.Must(session.NewSession(&aws.Config{
		Credentials: creds,
		Region:      aws.String(region),
	})), nil
}

// NewPrefix creates a new random S3 key prefix to anonymize uploads.
func NewPrefix() string {
	u, err := uuid.NewRandom()
	if err != nil {
		panic(err)
	}
	return u.String()
}

// Upload uploads the specified reader data to the specified S3 key.
func Upload(f io.Reader, s3Key string) error {
	sess, err := newSession()
	if err != nil {
		return xerrors.Errorf("Could not get session: %v", err)
	}
	uploader := s3manager.NewUploader(sess)
	_, err = uploader.Upload(&s3manager.UploadInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(s3Key),
		Body:   f,
	})
	if err != nil {
		return xerrors.Errorf("uploading to s3: %w", err)
	}
	return nil
}

// UploadFile uploads a file with the given name, creating a new key with
// a generated prefix.
func UploadFile(filename string) (string, error) {
	f, err := os.Open(filename)
	if err != nil {
		return "", xerrors.Errorf("opening file %q: %w", filename, err)
	}
	defer f.Close()
	s3Key := path.Join(NewPrefix(), filepath.Base(filename))
	return s3Key, Upload(f, s3Key)
}

// DeleteFile deletes the S3 file with the given key.
func DeleteFile(s3key string) error {
	sess, err := newSession()
	if err != nil {
		return xerrors.Errorf("Could not get session: %v", err)
	}
	svc := s3.New(sess)
	input := &s3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(s3key),
	}
	_, err = svc.DeleteObject(input)
	return err
}

// GetObjectTorrent returns the object metainfo for the given key.
func GetObjectTorrent(key string) (io.ReadCloser, error) {
	sess, err := newSession()
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
func GetTorrent(key string) error {
	t, err := GetObjectTorrent(key)
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
	*metainfo.MetaInfo
	os.FileInfo
	Err error
}

// IterUploads walks the torrent files stored in the directory.
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
		f(IteredUpload{MetaInfo: mi, FileInfo: e})
	}
	return nil
}
