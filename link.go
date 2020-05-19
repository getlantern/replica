package replica

import (
	"fmt"
	"net/url"
	"path"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
)

var s3BucketHttp = fmt.Sprintf("https://%s.s3-%s.amazonaws.com", bucket, region)

func CreateLink(ih torrent.InfoHash, s3Prefix S3Prefix, filePath []string) string {
	return metainfo.Magnet{
		InfoHash:    ih,
		DisplayName: path.Join(filePath...),
		Params: url.Values{
			"as": {fmt.Sprintf("%s/%s", s3BucketHttp, s3Prefix.TorrentKey())},
			"xs": {(&url.URL{Scheme: "replica", Opaque: s3Prefix.String()}).String()},
			// This might technically be more correct, but I couldn't find any torrent client that
			// supports it. Make sure to change any assumptions about "xs" before changing it.
			//"xs": {fmt.Sprintf("https://getlantern-replica.s3-ap-southeast-1.amazonaws.com/%s/torrent", s3Prefix)},

			// Since S3 key is provided, we know that it must be a single-file torrent.
			"so": {"0"},
			"ws": {
				fmt.Sprintf("%s/%s", s3BucketHttp, s3Prefix.FileDataKey(path.Join(filePath...))),
			},
		},
	}.String()
}

// See CreateLink.
func S3PrefixFromMagnet(m metainfo.Magnet) (S3Prefix, error) {
	// url.Parse("") doesn't return an error! (which is currently what we want here).
	u, err := url.Parse(m.Params.Get("xs"))
	if err != nil {
		return "", err
	}
	if u.Opaque != "" {
		return S3Prefix(u.Opaque), nil
	}
	return S3Prefix(u.Path), nil
}
