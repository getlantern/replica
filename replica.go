package replica

import (
	"fmt"
	"io"
	"log"
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
)

const bucket = "getlantern-replica"

func newSession() *session.Session {
	return session.Must(session.NewSession(&aws.Config{
		Region: aws.String("ap-southeast-1"),
	}))
}

func Upload(f io.Reader, s3Key string) error {
	sess := newSession()
	uploader := s3manager.NewUploader(sess)
	_, err := uploader.Upload(&s3manager.UploadInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(s3Key),
		Body:   f,
	})
	if err != nil {
		return xerrors.Errorf("uploading to s3: %w", err)
	}
	return nil
}

func UploadFile(filename string) (string, error) {
	f, err := os.Open(filename)
	if err != nil {
		return "", xerrors.Errorf("opening file %q: %w", filename, err)
	}
	defer f.Close()
	s3Key := filepath.Base(filename)
	return s3Key, Upload(f, s3Key)
}

func DeleteFile(s3key string) error {
	svc := s3.New(newSession())
	input := &s3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(s3key),
	}
	_, err := svc.DeleteObject(input)
	return err
}

// Returns the object metainfo for the given key.
func GetObjectTorrent(key string) (io.ReadCloser, error) {
	sess := newSession()
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

// Downloads the metainfo for the Replica object to a .torrent file in the current working directory.
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
	log.Printf("created %q", f.Name())
	defer f.Close()
	if _, err := io.Copy(f, t); err != nil {
		return xerrors.Errorf("copying torrent: %w", err)
	}
	if err := f.Close(); err != nil {
		return xerrors.Errorf("closing torrent file: %w", f.Close())
	}
	return nil
}

// Walks the torrent files stored in the directory.
func IterUploads(dir string, f func(mi *metainfo.MetaInfo, err error)) error {
	return filepath.Walk(dir, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			f(nil, err)
			return nil
		}
		if fi.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".torrent" {
			return nil
		}
		mi, err := metainfo.LoadFromFile(path)
		if err != nil {
			f(nil, fmt.Errorf("error loading metainfo from file %q: %w", path, err))
			return nil
		}
		f(mi, nil)
		return nil
	})
}
