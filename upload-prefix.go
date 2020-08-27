package replica

import (
	"fmt"
	"path"

	"github.com/google/uuid"
)

type UploadPrefix struct {
	Prefix
}

type Prefix interface {
	PrefixString() string
}

type UUIDPrefix struct {
	uuid.UUID
}

func (me UUIDPrefix) PrefixString() string {
	return me.UUID.String()
}

type ProviderPrefix struct {
	provider string
	id       string
}

func (me ProviderPrefix) PrefixString() string {
	return fmt.Sprintf("%v-%v", me.provider, me.id)
}

func (me UploadPrefix) String() string {
	return me.PrefixString()
}

// TorrentKey returns the key where the metainfo for the data directory should be stored.
func (me UploadPrefix) TorrentKey() string {
	return path.Join(me.PrefixString(), "torrent")
}

func (me UploadPrefix) DataKey() string {
	return path.Join(me.PrefixString(), "data")
}
