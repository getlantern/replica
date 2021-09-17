package replica

import (
	"errors"
	"fmt"
	"net/url"
	"path"
	"path/filepath"
	"strings"

	"github.com/anacrolix/torrent/metainfo"
	"github.com/google/uuid"
)

// Upload is information pertaining to a torrent uploaded to Replica.
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

func ExactSource(infoName Prefix) string {
	return (&url.URL{
		Scheme: "replica",
		Opaque: infoName.PrefixString(),
	}).String()
}

// UploadPrefix is a prefix specific to a Replica upload (probably a bit unnecessary now).
type UploadPrefix struct {
	Prefix
}

// Prefix is the identifying first directory component for the Replica S3 key layout.
type Prefix string

func NewUuidPrefix() Prefix {
	return Prefix(uuid.New().String())
}

func (p Prefix) PrefixString() string {
	return string(p)
}

func (p Prefix) String() string {
	return string(p)
}

// TODO: Could do checks to ensure prefixes are now infohashes.
func UploadPrefixFromString(s string) UploadPrefix {
	return UploadPrefix{Prefix(s)}
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

func (me *UploadMetainfo) FromTorrentMetainfo(mi *metainfo.MetaInfo, fileName string) error {
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
		// Note that here we are passing the *file stem* as the prefix, to match with the upload
		// admin token in the user's upload directory.
		Upload: Upload{UploadPrefix{Prefix(strings.TrimSuffix(fileName, filepath.Ext(fileName)))}},
	}
	return nil
}

func (me UploadMetainfo) FilePath() []string {
	return me.Info.UpvertedFiles()[0].Path
}
