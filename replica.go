package replica

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	_ "github.com/anacrolix/envpprof"
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

func UploadFile(filename string) error {
	sess := newSession()

	// Create an uploader with the session and default options
	uploader := s3manager.NewUploader(sess)

	f, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("failed to open file %q: %v", filename, err)
	}

	s3Key := filepath.Base(filename)

	// Upload the file to S3.
	result, err := uploader.Upload(&s3manager.UploadInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(s3Key),
		Body:   f,
	})
	if err != nil {
		return xerrors.Errorf("failed to upload file, %w", err)
	}
	log.Printf("file uploaded to %q\n", result.Location)
	fmt.Println(s3Key)
	return nil
}

func GetTorrent(filename string) error {
	sess := newSession()
	svc := s3.New(sess)
	out, err := svc.GetObjectTorrent(&s3.GetObjectTorrentInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(filename),
	})
	if err != nil {
		return err
	}
	defer out.Body.Close()
	f, err := os.OpenFile(filepath.Base(filename)+".torrent", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0640)
	if err != nil {
		return xerrors.Errorf("opening output file: %w", err)
	}
	log.Printf("created %q", f.Name())
	defer f.Close()
	if _, err := io.Copy(f, out.Body); err != nil {
		return xerrors.Errorf("copying torrent: %w", err)
	}
	if err := f.Close(); err != nil {
		return xerrors.Errorf("closing torrent file: %w", f.Close())
	}
	return nil
}
