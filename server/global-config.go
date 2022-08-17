package server

import "github.com/getlantern/replica/service"

// We're not coupled to Flashlight's global config, but the concept originated there and using that name should get
// everyone on the same page the quickest.

type ReplicaOptions interface {
	// Use infohash and old-style prefixing simultaneously for now. Later, the old-style can be removed.
	GetWebseedBaseUrls() []string
	GetTrackers() []string
	GetStaticPeerAddrs() []string
	// Merged with the webseed URLs when the metadata and data buckets are merged.
	GetMetadataBaseUrls() []string
	// The replica-rust endpoint to use. There's only one because object uploads and ownership are
	// fixed to a specific bucket, and replica-rust endpoints are 1:1 with a bucket.
	GetReplicaRustEndpoint() string
	// The CA to use when communicating with the Replica service (commonly known as replica-rust). If empty, the default
	// CA is used.
	GetCustomCA() string
}

// These are the default options for Replica when the config is not available via feature options. Maybe consider what older
// clients that don't know about Replica feature options would use for these values, though there could be more up to
// date values to provide here.
type FallbackReplicaOptions struct{}

func (f FallbackReplicaOptions) GetWebseedBaseUrls() []string {
	return nil
}

func (f FallbackReplicaOptions) GetTrackers() []string {
	return nil
}

func (f FallbackReplicaOptions) GetStaticPeerAddrs() []string {
	return nil
}

func (f FallbackReplicaOptions) GetMetadataBaseUrls() []string {
	return nil
}

func (f FallbackReplicaOptions) GetReplicaRustEndpoint() string {
	return service.GlobalChinaDefaultServiceUrl.String()
}

func (f FallbackReplicaOptions) GetCustomCA() string {
	return ""
}
