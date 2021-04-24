package replica

import (
	"fmt"
	"net/http"
)

type AnyStorageClientParams struct {
	HttpClient       *http.Client
	TencentSecretId  string
	TencentSecretKey string
}

func StorageClientForEndpoint(e Endpoint, params AnyStorageClientParams) (_ StorageClient, err error) {
	providerParam := e.LinkParams().Get(ProviderEndpointKey)
	if providerParam == "" {
		providerParam = DefaultEndpoint.LinkParams().Get(ProviderEndpointKey)
	}
	DefaultEndpoint.LinkParams()
	switch providerParam {
	case StorageProviderS3:
		return NewS3StorageClient(
			e.LinkParams().Get(BucketEndpointKey),
			e.LinkParams().Get(RegionEndpointKey),
			params.HttpClient), nil
	case StorageProviderTencent:
		return NewTencentStorage(
			params.TencentSecretId,
			params.TencentSecretKey,
			e.LinkParams().Get(BucketEndpointKey),
			e.LinkParams().Get(RegionEndpointKey))
	default:
		err = fmt.Errorf("unhandled provider %q", providerParam)
		return
	}
}
