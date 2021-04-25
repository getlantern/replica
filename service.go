package replica

import (
	"net/url"
	"path"
)

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
