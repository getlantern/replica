package replica

import (
	"fmt"
	"net/url"
	"path"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
)

func (r *Endpoint) rootUrls() []string {
	return []string{
		// Virtual-hosted-style
		fmt.Sprintf("https://%s.s3.%s.amazonaws.com", r.BucketName, r.AwsRegion),
		// Path-style
		fmt.Sprintf("https://s3.%s.amazonaws.com/%s", r.AwsRegion, r.BucketName),
	}
}

func CreateLink(ih torrent.InfoHash, s3upload Upload, filePath []string) string {
	return metainfo.Magnet{
		InfoHash:    ih,
		DisplayName: path.Join(filePath...),
		Params: url.Values{
			"as": s3upload.MetainfoUrls(),
			"xs": {s3upload.ExactSource()},
			// This might technically be more correct, but I couldn't find any torrent client that
			// supports it. Make sure to change any assumptions about "xs" before changing it.
			//"xs": {fmt.Sprintf("https://getlantern-replica.s3-ap-southeast-1.amazonaws.com/%s/torrent", s3upload)},

			// Since S3 key is provided, we know that it must be a single-file torrent.
			"so": {"0"},
			"ws": s3upload.WebseedUrls(),
		},
	}.String()
}

// See CreateLink.
func (upload *Upload) FromMagnet(m metainfo.Magnet) error {
	return upload.FromExactSource(m.Params.Get("xs"))
}
