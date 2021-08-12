package replica

import (
	"net/url"
	"path"
)

// This exists for anything that doesn't have configuration but expects to
// connect to an arbitrary replica-rust service. At least in flashlight, this
// is provided by configuration instead.
const DefaultReplicaRustEndpoint string = "https://replica-search.lantern.io"

type ServiceUploadOutput struct {
	Link       string `json:"link"`
	Metainfo   string `json:"metainfo"`
	AdminToken string `json:"admin_token"`
}

// Completes the upload endpoint URL with the file-name, per the replica-rust upload endpoint API.
func serviceUploadUrl(fetchBaseUrlFunc func() *url.URL, fileName string) *url.URL {
	return fetchBaseUrlFunc().
		ResolveReference(&url.URL{Path: path.Join("upload", fileName)})
}

func serviceDeleteUrl(fetchBaseUrlFunc func() *url.URL) *url.URL {
	return fetchBaseUrlFunc().
		ResolveReference(&url.URL{Path: "delete"})
}
