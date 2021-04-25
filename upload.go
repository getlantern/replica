package replica

import (
	"errors"
	"fmt"
	"net/url"
	"path"
	"path/filepath"

	"github.com/anacrolix/torrent/metainfo"
	"github.com/google/uuid"
)

// Upload is the UUID or provider+id prefix used on cloud object storage to group objects related to an upload.
type Upload struct {
	UploadPrefix
	Endpoint
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

	query := u.Query()

	endpoint := Endpoint{
		BucketName: query.Get("bucket"),
		Region:     query.Get("region"),
	}

	uploadPrefix := UploadPrefixFromString(u.Opaque)

	*me = Upload{
		UploadPrefix: uploadPrefix,
		Endpoint:     endpoint,
	}
	return nil
}

func (me Upload) FileDataKey(
	// These are the path components per "github.com/anacrolix/torrent/metainfo".Info.Files.Path
	filePathComps ...string,
) string {
	return path.Join(append([]string{me.DataKey(), me.PrefixString()}, filePathComps...)...)
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
		Opaque: me.UploadPrefix.String(),
		RawQuery: url.Values{
			"bucket": {me.BucketName},
			"region": {me.Region},
		}.Encode(),
	}).String()
}

// UploadConfig provides config information for the upload
type UploadConfig interface {
	Filename() string
	FullPath() string
	GetPrefix() UploadPrefix
}

// ProviderUploadConfig is a type of UploadConfig which has the format of provider + id prefix
type ProviderUploadConfig struct {
	File       string
	ProviderID string
	Name       string
}

func (pc *ProviderUploadConfig) FullPath() string {
	return pc.File
}

func (pc *ProviderUploadConfig) Filename() string {
	if pc.Name != "" {
		return pc.Name
	}
	return filepath.Base(pc.File)
}

func (pc *ProviderUploadConfig) GetPrefix() UploadPrefix {
	return UploadPrefix{ProviderPrefix{
		providerID: pc.ProviderID,
	}}
}

type uuidUploadConfig struct {
	file string
	uuid uuid.UUID
	name string
}

// NewUUIDUploadConfig creates a new uuidUploadConfig which implements the UploadConfig interface
// The first parameter f will be used strictly for potentially opening a local file and can be left blank
// The second parameter n will be used as the name of the upload. If it is left blank, the name of the
// upload will come from the first parameter's "base" name
func NewUUIDUploadConfig(f, n string) *uuidUploadConfig {
	u := uuid.New()
	return &uuidUploadConfig{file: f, uuid: u, name: n}
}

func (uc *uuidUploadConfig) FullPath() string {
	return uc.file
}

func (uc *uuidUploadConfig) Filename() string {
	if uc.name != "" {
		return uc.name
	}
	return filepath.Base(uc.file)
}

func (uc *uuidUploadConfig) GetPrefix() UploadPrefix {
	return UploadPrefix{UUIDPrefix{uc.uuid}}
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
func (me UploadPrefix) TorrentKey() string {
	return path.Join(me.PrefixString(), "torrent")
}

func (me UploadPrefix) DataKey() string {
	return path.Join(me.PrefixString(), "data")
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
			// We assume the default endpoint, because that's the one that was in use when this
			// comment-style was standard. If the default endpoint changes, this should probably be
			// changed to reflect where "Replica"-comment uploads would now reside.
			Endpoint: DefaultEndpoint,
		}
		return nil
	default:
	}
	return me.Upload.FromExactSource(mi.Comment)
}

func (me UploadMetainfo) FilePath() []string {
	return me.Info.UpvertedFiles()[0].Path
}
