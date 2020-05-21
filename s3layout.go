package replica

import (
	"path"

	"github.com/google/uuid"
)

// NewPrefix creates a new random S3 key prefix to anonymize uploads.
func NewPrefix() S3Prefix {
	u, err := uuid.NewRandom()
	if err != nil {
		panic(err)
	}
	return S3Prefix(u.String())
}

// S3Prefix is the UUID prefix used on S3 to group objects related to an upload.
type S3Prefix string

func (me S3Prefix) String() string {
	return string(me)
}

// TorrentKey returns the key where the metainfo for the data directory should be stored.
func (me S3Prefix) TorrentKey() string {
	return path.Join(string(me), "torrent")
}

func (me S3Prefix) DataKey() string {
	return path.Join(string(me), "data")
}

func (me S3Prefix) FileDataKey(
	// These are the path components per "github.com/anacrolix/torrent/metainfo".Info.Files.Path
	filePathComps ...string,
) string {
	return path.Join(append([]string{me.DataKey(), me.String()}, filePathComps...)...)
}
