package replica

import (
	"fmt"
	"net/url"
	"path"

	"github.com/google/uuid"
)

// Upload is the UUID prefix used on S3 to group objects related to an upload.
type Upload struct {
	UploadPrefix
	Endpoint
}

func (me *Upload) FromExactSource(s string) error {
	u, err := url.Parse(s)
	if err != nil {
		return fmt.Errorf("parsing url: %w", err)
	}
	uuid, err := uuid.Parse(u.Opaque)
	if err != nil {
		return fmt.Errorf("parsing uuid: %w", err)
	}
	query := u.Query()
	*me = Upload{
		UploadPrefix: UploadPrefix{uuid},
		Endpoint: Endpoint{
			BucketName: query.Get("bucket"),
			AwsRegion:  query.Get("region"),
		},
	}
	return nil
}

func (me Upload) FileDataKey(
	// These are the path components per "github.com/anacrolix/torrent/metainfo".Info.Files.Path
	filePathComps ...string,
) string {
	return path.Join(append([]string{me.DataKey(), me.String()}, filePathComps...)...)
}

func (me Upload) mapAppendRootUrls(suffix string) (ret []string) {
	for _, root := range me.rootUrls() {
		ret = append(ret, root+suffix)
	}
	return
}

func (me Upload) WebseedUrls() []string {
	return me.mapAppendRootUrls(fmt.Sprintf("/%s/", me.DataKey()))
}

func (me Upload) MetainfoUrls() []string {
	return me.mapAppendRootUrls(fmt.Sprintf("/%s", me.TorrentKey()))

}

func (me Upload) ExactSource() string {
	return (&url.URL{
		Scheme: "replica",
		Opaque: me.UUID.String(),
		RawQuery: url.Values{
			"bucket": {me.BucketName},
			"region": {me.AwsRegion},
		}.Encode(),
	}).String()
}
