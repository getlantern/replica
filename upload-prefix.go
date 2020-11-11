package replica

import (
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
	providerID string
}

func (me ProviderPrefix) PrefixString() string {
	return me.providerID
}

// UploadPrefixFromString creates a ProviderPrefix or a UUIDPrefix depending on the format of the provided string
func UploadPrefixFromString(s string) UploadPrefix {
	var uploadPrefix UploadPrefix

	uuid, err := uuid.Parse(s)
	if err == nil {
		uploadPrefix = UploadPrefix{UUIDPrefix{uuid}}
	} else {
		uploadPrefix = UploadPrefix{ProviderPrefix{s}}
	}

	return uploadPrefix
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
