package replica

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/mozillazg/go-cos"
)

func NewTencentStorage(secretID, secretKey, bucketName, region string) (_ *tencentStorage, err error) {
	cosObjectService, err := newTencentCosClient(
		bucketName, region,
		// TODO: Do we need to allow passing proxies in here?
		&http.Client{
			Transport: &cos.AuthorizationTransport{
				SecretID:  secretID,
				SecretKey: secretKey,
			}})
	if err != nil {
		return
	}
	return &tencentStorage{cosObjectService}, nil
}

type tencentStorage struct {
	cosObjectService *cos.ObjectService
}

func (s *tencentStorage) Get(key string) (io.ReadCloser, error) {
	resp, err := s.cosObjectService.Get(context.Background(), key, nil)
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

func (s *tencentStorage) Put(key string, r io.Reader) error {
	_, err := s.cosObjectService.Put(context.Background(), key, r, nil)
	return err
}

func (s *tencentStorage) Delete(key string) error {
	_, err := s.cosObjectService.Delete(context.Background(), key)
	return err
}

func newTencentCosClient(bucketName, region string, httpClient *http.Client) (_ *cos.ObjectService, err error) {
	b, err := cos.NewBaseURL(fmt.Sprintf("https://%s.cos.%s.myqcloud.com", bucketName, region))
	if err != nil {
		return
	}
	return cos.NewClient(b, httpClient).Object, nil
}
