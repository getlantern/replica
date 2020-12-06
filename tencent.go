package replica

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/mozillazg/go-cos"
)

func NewTencentStorage(secretID, secretKey string) Storage {
	return &tencentStorage{&http.Client{
		Transport: &cos.AuthorizationTransport{
			SecretID:  secretID,
			SecretKey: secretKey,
		}}}
}

type tencentStorage struct {
	httpClient *http.Client
}

func (s *tencentStorage) Get(endpoint Endpoint, key string) (io.ReadCloser, error) {
	sess, err := s.newSession(endpoint)
	if err != nil {
		return nil, err
	}
	resp, err := sess.Get(context.Background(), key, nil)
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

func (s *tencentStorage) Put(endpoint Endpoint, key string, r io.Reader) error {
	sess, err := s.newSession(endpoint)
	if err != nil {
		return err
	}
	_, err = sess.Put(context.Background(), key, r, nil)
	return err
}

func (s *tencentStorage) Delete(endpoint Endpoint, key string) error {
	sess, err := s.newSession(endpoint)
	if err != nil {
		return err
	}
	_, err = sess.Delete(context.Background(), key)
	return err
}

func (s *tencentStorage) newSession(endpoint Endpoint) (*cos.ObjectService, error) {
	b, err := cos.NewBaseURL(fmt.Sprintf("https://%s.cos.%s.myqcloud.com", endpoint.BucketName, endpoint.Region))
	if err != nil {
		return nil, err
	}
	return cos.NewClient(b, s.httpClient).Object, nil
}
