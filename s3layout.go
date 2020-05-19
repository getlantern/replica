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

// The UUID prefix used on S3 to group objects related to an upload.
type S3Prefix string

func (me S3Prefix) String() string {
	return string(me)
}

// The key where the metainfo for the data directory should be stored.
func (me S3Prefix) TorrentKey() string {
	return path.Join(string(me), "torrent")
}

func (me S3Prefix) DataKey() string {
	return path.Join(string(me), "data")
}

func (me S3Prefix) FileDataKey(filePath string) string {
	return path.Join(me.DataKey(), me.String(), filePath)
}
