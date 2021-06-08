package replica

import (
	"errors"
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
func (upload *Upload) FromMagnet(m metainfo.Magnet) error {
	return upload.FromExactSource(m.Params.Get("xs"))
}

// Parses the content of the "xs" magnet link field, which is also the metainfo "comment" field in
// newer uploads.
func (me *Upload) FromExactSource(s string) error {
	u, err := url.Parse(s)
	if err != nil {
		return fmt.Errorf("parsing url: %w", err)
	}

	if u.Opaque == "" {
		return errors.New("exact source url opaque value must not be empty")
	}

	uploadPrefix := UploadPrefixFromString(u.Opaque)

	*me = Upload{
		UploadPrefix: uploadPrefix,
	}
	return nil
}

func (me Upload) FileDataKey(
	prefix string,
	// These are the path components per "github.com/anacrolix/torrent/metainfo".Info.Files.Path
	filePathComps ...string,
) string {
	return path.Join(append([]string{DataKey(prefix), me.PrefixString()}, filePathComps...)...)
}

func ExactSource(p Prefix) string {
	return (&url.URL{
		Scheme: "replica",
		Opaque: p.PrefixString(),
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

// UploadPrefixFromString creates a ProviderPrefix or a UUIDPrefix depending on
// the format of the provided string
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

func (me *UploadMetainfo) FromTorrentMetainfo(mi *metainfo.MetaInfo) error {
	info, err := mi.UnmarshalInfo()
	if err != nil {
		return fmt.Errorf("unmarshalling info: %w", err)
	}
	if len(info.UpvertedFiles()) != 1 {
		return errors.New("expected single file")
	}
	*me = UploadMetainfo{
		MetaInfo: mi,
		Info:     info,
	}
	switch mi.Comment {
	//case "":
	//// There should be *no* torrent uploads with no comment. However if it did happen, we could
	//// recover if the torrent name was a UUID. A provider would accept any string, so it's not clear
	//// if we want to follow that as a possibility.
	//fallthrough
	case "Replica":
		// A long time ago, we uploaded with this as the comment, and there were only UUID prefixes.
		u, err := uuid.Parse(info.Name)
		if err != nil {
			return fmt.Errorf("parsing uuid from info name: %w", err)
		}
		me.Upload = Upload{
			UploadPrefix: UploadPrefix{UUIDPrefix{u}},
		}
		return nil
	default:
	}
	return me.Upload.FromExactSource(mi.Comment)
}

func (me UploadMetainfo) FilePath() []string {
	return me.Info.UpvertedFiles()[0].Path
}
