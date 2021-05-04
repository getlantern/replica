package replica

import (
	"fmt"
	"net/http"
	"net/url"
)

const (
	// Used in replica "xs" provider field
	StorageProviderS3      = "s3"
	StorageProviderTencent = "tencent"

	// Keys in the "xs" url values
	ProviderEndpointKey = "provider"
	BucketEndpointKey   = "bucket"
	RegionEndpointKey   = "region"

	// These should probably be extractable from the default endpoint instead, since that includes
	// the provider type.
	DefaultBucket = "getlantern-replica"
	DefaultRegion = "ap-southeast-1"
)

var (
	DefaultHttpClient            = http.DefaultClient
	DefaultServiceUrl            = &url.URL{Scheme: "https", Host: "replica-search.lantern.io"}
	DefaultEndpoint              = NewS3Endpoint(DefaultBucket, DefaultRegion)
	DefaultMetadataEndpoint      = NewS3Endpoint("replica-metadata", "ap-southeast-1")
	DefaultMetadataStorageClient = func() StorageClient {
		ret, err := StorageClientForEndpoint(DefaultMetadataEndpoint, AnyStorageClientParams{HttpClient: DefaultHttpClient})
		if err != nil {
			panic(err)
		}
		return ret
	}()
	DefaultClient = Client{
		StorageClient: NewS3StorageClient(DefaultBucket, DefaultRegion, DefaultHttpClient),
		Endpoint:      DefaultEndpoint,
		ServiceClient: ServiceClient{
			HttpClient:             DefaultHttpClient,
			ReplicaServiceEndpoint: DefaultServiceUrl,
		},
	}
)

type Endpoint interface {
	RootUrls() []string
	LinkParams() url.Values
}

type s3CompatibleStorageProvider struct {
	ProviderTypeId               string
	BucketName, Region           string
	RootDomain, ServiceSubDomain string
}

func (r s3CompatibleStorageProvider) RootUrls() []string {
	templateArgs := []interface{}{r.BucketName, r.ServiceSubDomain, r.Region, r.RootDomain}
	return []string{
		// Virtual-hosted-style
		fmt.Sprintf("https://%[1]s.%[2]s.%[3]s.%[4]s", templateArgs...),
		// Path-style
		fmt.Sprintf("https://%[2]s.%[3]s.%[4]s/%[1]s",
			// See https://github.com/golang/go/issues/45742 for why we can't use templateArgs :s
			r.BucketName, r.ServiceSubDomain, r.Region, r.RootDomain),
	}
}

func (me s3CompatibleStorageProvider) LinkParams() url.Values {
	return url.Values{
		ProviderEndpointKey: {me.ProviderTypeId},
		BucketEndpointKey:   {me.BucketName},
		RegionEndpointKey:   {me.Region},
	}
}

func NewS3Endpoint(bucketName, region string) Endpoint {
	return s3CompatibleStorageProvider{
		ProviderTypeId:   StorageProviderS3,
		BucketName:       bucketName,
		Region:           region,
		RootDomain:       "amazonaws.com",
		ServiceSubDomain: "s3",
	}
}

func NewTencentEndpoint(bucketName, region string) Endpoint {
	return s3CompatibleStorageProvider{
		ProviderTypeId:   StorageProviderTencent,
		BucketName:       bucketName,
		Region:           region,
		RootDomain:       "myqcloud.com",
		ServiceSubDomain: "cos",
	}

}

// NewUpload creates a new random uuid or provider+id S3 key prefix to anonymize uploads.
func NewUpload(uConfig UploadConfig, r Endpoint) Upload {
	return Upload{
		UploadPrefix: uConfig.GetPrefix(),
		Endpoint:     r,
	}
}
