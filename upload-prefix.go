package replica

import (
	"path"

	"github.com/google/uuid"
)

type UploadPrefix struct {
	uuid.UUID
}

func (me UploadPrefix) String() string {
	return me.UUID.String()
}

// TorrentKey returns the key where the metainfo for the data directory should be stored.
func (me UploadPrefix) TorrentKey() string {
	return path.Join(me.UUID.String(), "torrent")
}

func (me UploadPrefix) DataKey() string {
	return path.Join(me.UUID.String(), "data")
}
