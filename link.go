package replica

import (
	"net/url"
	"path"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/getlantern/replica/service"
)

func CreateLink(ih torrent.InfoHash, infoName service.Prefix, filePath []string) string {
	return metainfo.Magnet{
		InfoHash:    ih,
		DisplayName: path.Join(filePath...),
		Params: url.Values{
			"xs": {service.ExactSource(infoName)},
			// Since S3 key is provided, we know that it must be a single-file torrent.
			"so": {"0"},
		},
	}.String()
}
