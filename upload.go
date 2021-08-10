package replica

import (
	"fmt"
	"net/url"
	"path"

	"github.com/anacrolix/torrent/metainfo"
	"github.com/google/uuid"
)

// Upload is the UUID or provider+id prefix used on cloud object storage to group objects related to an upload.
type Upload struct {
	UploadPrefix
}

// See CreateLink.
func NewUploadFromMagnetLink(m metainfo.Magnet) (Upload, error) {
	// FIXME
	// return NewUploadFromExactSource("replica:" + m.InfoHash.HexString())
	return NewUploadFromExactSource(m.Params.Get("xs"))
}

// Parses the content of the "xs" magnet link field, which is also the metainfo
// "comment" field in newer uploads.
func NewUploadFromExactSource(s string) (Upload, error) {
	u, err := url.Parse(s)
	if err != nil {
		return Upload{}, fmt.Errorf("parsing url: %w", err)
	}
	if u.Opaque == "" {
		return Upload{}, fmt.Errorf("exact source url opaque value must not be empty")
	}
	return Upload{UploadPrefix: NewUploadPrefixFromString(u.Opaque)}, nil
}

func (me Upload) FileDataKey(
	prefix string,
	// These are the path components per "github.com/anacrolix/torrent/metainfo".Info.Files.Path
	filePathComps ...string,
) string {
	return path.Join(append([]string{DataKey(prefix), me.PrefixString()}, filePathComps...)...)
}

func ExactSource(infoName Prefix) string {
	return (&url.URL{
		Scheme: "replica",
		Opaque: infoName.PrefixString(),
	}).String()
}

type UploadPrefix struct {
	Prefix
}

type Prefix interface {
	PrefixString() string
}

type UUIDPrefix struct {
	uuid.UUID
}

func NewUuidPrefix() UUIDPrefix {
	return UUIDPrefix{uuid.New()}
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

// NewUploadPrefixFromString creates a ProviderPrefix or a UUIDPrefix depending
// on the format of the provided string
// FIXME remove this UUIDPrefix business
func NewUploadPrefixFromString(s string) UploadPrefix {
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
func TorrentKey(prefix string) string {
	return path.Join(prefix, "torrent")
}

func DataKey(prefix string) string {
	return path.Join(prefix, "data")
}

// Wraps an upload metainfo.Metainfo with Replica-related values parsed out.
type UploadMetainfo struct {
	*metainfo.MetaInfo
	// TODO: When Go finally gets an optional type, this should be handled by a helper, as it may or
	// may not be created as a side-effect of uploading (for example uploading directly requires
	// that the info is produced locally).
	metainfo.Info
	Upload
}

// NewUploadMetainfo creates a new uploadMetainfo from metainfo
func NewUploadMetainfo(mi *metainfo.MetaInfo) (UploadMetainfo, error) {
	info, err := mi.UnmarshalInfo()
	if err != nil {
		return UploadMetainfo{}, fmt.Errorf("unmarshalling info: %w", err)
	}
	if len(info.UpvertedFiles()) != 1 {
		return UploadMetainfo{}, fmt.Errorf("expected single file")
	}
	return UploadMetainfo{
		MetaInfo: mi,
		Info:     info,
		Upload:   Upload{NewUploadPrefixFromString(mi.HashInfoBytes().HexString())},
	}, nil
}

func (me UploadMetainfo) FilePath() []string {
	return me.Info.UpvertedFiles()[0].Path
}
