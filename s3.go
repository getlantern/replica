package replica

import (
	"fmt"
	"io"
	"net/http"

	"github.com/aws/aws-sdk-go/aws"
	session "github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
)

var credsProvider cognitoProvider

func NewS3StorageClient(bucketName, region string, httpClient *http.Client) S3Storage {
	return S3Storage{
		Region:     region,
		HttpClient: httpClient,
		BucketName: bucketName,
	}
}

type S3Storage struct {
	Region     string
	HttpClient *http.Client
	BucketName string
}

func (s S3Storage) Get(key string) (_ io.ReadCloser, err error) {
	sess, err := s.newSession()
	if err != nil {
		return
	}
	out, err := s3.New(sess).GetObject(&s3.GetObjectInput{
		Bucket: aws.String(s.BucketName),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("getting s3 object: %w", err)
	}
	return out.Body, nil
}

func (s S3Storage) Put(key string, r io.Reader) error {
	sess, err := s.newSession()
	if err != nil {
		return err
	}
	_, err = s3manager.NewUploader(sess).Upload(&s3manager.UploadInput{
		Bucket: aws.String(s.BucketName),
		Key:    aws.String(key),
		Body:   r,
	})
	return err
}

func (s S3Storage) Delete(key string) error {
	sess, err := s.newSession()
	if err != nil {
		return err
	}
	_, err = s3.New(sess).DeleteObject(&s3.DeleteObjectInput{
		Bucket: aws.String(s.BucketName),
		Key:    aws.String(key),
	})
	return err
}

func (s S3Storage) newSession() (_ *session.Session, err error) {
	return newS3Session(s.Region, s.HttpClient)
}

func newS3Session(region string, httpClient *http.Client) (_ *session.Session, err error) {
	creds, err := credsProvider.getCredentials(region)
	if err != nil {
		err = fmt.Errorf("getting creds: %w", err)
	}
	sess := session.Must(session.NewSession(&aws.Config{
		Credentials:      creds,
		Region:           aws.String(region),
		HTTPClient:       httpClient,
		S3ForcePathStyle: aws.Bool(true),
	}))
	return sess, nil
}
