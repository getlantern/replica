package replica

import (
	"net/url"
)

type RegionClient struct {
	MetadataStorage StorageClient
	UploadClient    Client
}

type RegionParams struct {
	MetadataEndpoint Endpoint
	UploadEndpoint   Endpoint
	ServiceUrl       *url.URL
}

var (
	GlobalChinaRegionParams = RegionParams{
		MetadataEndpoint: NewS3Endpoint("replica-metadata", "ap-southeast-1"),
		UploadEndpoint:   NewS3Endpoint("getlantern-replica", "ap-southeast-1"),
		ServiceUrl:       &url.URL{Scheme: "https", Host: "replica-search.lantern.io"},
	}
	IranRegionParams = RegionParams{
		MetadataEndpoint: NewS3Endpoint("replica-metadata-frankfurt", "eu-central-1"),
		UploadEndpoint:   NewS3Endpoint("replica-search-frankfurt", "eu-central-1"),
		ServiceUrl:       &url.URL{Scheme: "https", Host: "replica-search-aws.lantern.io"},
	}
)
