package replica

import (
	"fmt"
	"io"
	"net/http"

	"github.com/aws/aws-sdk-go/aws"
	session "github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"golang.org/x/xerrors"
)

var credsProvider cognitoProvider

type S3Storage struct {
	HttpClient *http.Client
}

func (s S3Storage) Get(endpoint Endpoint, key string) (io.ReadCloser, error) {
	sess, err := s.newSession(endpoint.Region)
	if err != nil {
		return nil, err
	}
	cl := s3.New(sess)
	out, err := cl.GetObject(&s3.GetObjectInput{
		Bucket: aws.String(endpoint.BucketName),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("getting s3 object: %w", err)
	}
	return out.Body, nil
}

func (s S3Storage) Put(endpoint Endpoint, key string, r io.Reader) error {
	sess, err := s.newSession(endpoint.Region)
	if err != nil {
		return err
	}
	uploader := s3manager.NewUploader(sess)
	_, err = uploader.Upload(&s3manager.UploadInput{
		Bucket: aws.String(endpoint.BucketName),
		Key:    aws.String(key),
		Body:   r,
	})
	return err
}

func (s S3Storage) Delete(endpoint Endpoint, key string) error {
	sess, err := s.newSession(endpoint.Region)
	if err != nil {
		return err
	}
	svc := s3.New(sess)
	_, err = svc.DeleteObject(&s3.DeleteObjectInput{
		Bucket: aws.String(endpoint.BucketName),
		Key:    aws.String(key),
	})
	return err
}

func (s S3Storage) newSession(region string) (*session.Session, error) {
	creds, err := credsProvider.getCredentials(region)
	if err != nil {
		return nil, xerrors.Errorf("could not get creds: %v", err)
	}
	sess := session.Must(session.NewSession(&aws.Config{
		Credentials:      creds,
		Region:           aws.String(region),
		HTTPClient:       s.HttpClient,
		S3ForcePathStyle: aws.Bool(true),
	}))
	return sess, nil
}
