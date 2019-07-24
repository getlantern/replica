package main

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"golang.org/x/xerrors"
)

const bucket = "getlantern-replica"

func main() {
	err := mainErr()
	if err != nil {
		log.Fatal(err)
	}
}

func mainErr() error {
	switch os.Args[1] {
	case "upload":
		return uploadFile(os.Args[2])
	case "get-torrent":
		return getTorrent(os.Args[2])
	default:
		return errors.New("unknown command")
	}
}

func upload(args []string) error {
	return uploadFile(args[0])
}

func uploadFile(filename string) error {
	// The session the S3 Uploader will use
	sess := session.Must(session.NewSession())

	// Create an uploader with the session and default options
	uploader := s3manager.NewUploader(sess)

	f, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("failed to open file %q, %v", filename, err)
	}

	// Upload the file to S3.
	result, err := uploader.Upload(&s3manager.UploadInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(filename),
		Body:   f,
	})
	if err != nil {
		return fmt.Errorf("failed to upload file, %v", err)
	}
	log.Printf("file uploaded to, %s\n", result.Location)
	return nil
}

func getTorrent(filename string) error {
	sess := session.Must(session.NewSession())
	svc := s3.New(sess)
	out, err := svc.GetObjectTorrent(&s3.GetObjectTorrentInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(filename),
	})
	if err != nil {
		return err
	}
	defer out.Body.Close()
	f, err := os.OpenFile(filename+".torrent", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0750)
	if err != nil {
		return xerrors.Errorf("opening output file: %w", err)
	}
	defer f.Close()
	if _, err := io.Copy(f, out.Body); err != nil {
		return xerrors.Errorf("copying torrent: %w", err)
	}
	if err := f.Close(); err != nil {
		return xerrors.Errorf("closing torrent file: %w", f.Close())
	}
	return nil
}
