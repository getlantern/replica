package replica

import (
	"net/url"
	"path"
)

// This exists for anything that doesn't have configuration but expects to connect to an arbitrary
// replica-rust service. At least in flashlight, this is provided by configuration instead.
var GlobalChinaDefaultServiceUrl = &url.URL{
	Scheme: "https",
	Host:   "replica-search.lantern.io",
}

// Interface to the replica-rust/"Replica service".

type ServiceUploadOutput struct {
	Link       string `json:"link"`
	Metainfo   string `json:"metainfo"`
	AdminToken string `json:"admin_token"`
}

// Completes the upload endpoint URL with the file-name, per the replica-rust upload endpoint API.
func serviceUploadUrl(base *url.URL, fileName string) *url.URL {
	return base.ResolveReference(&url.URL{Path: path.Join("upload", fileName)})
}

func serviceDeleteUrl(base *url.URL) *url.URL {
	return base.ResolveReference(&url.URL{Path: "delete"})
}
