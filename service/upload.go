package service

import (
	"errors"
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
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

type IteredUpload struct {
	Metainfo UploadMetainfo
	FileInfo os.FileInfo
	Err      error
}

// IterUploads walks the torrent files (UUID-uploads?) stored in the directory. This is specific to
// the replica desktop server, except that maybe there is replica-project specific stuff to extract
// from metainfos etc. The prefixes are the upload file stems.
func IterUploads(dir string, f func(IteredUpload)) error {
	entries, err := ioutil.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, e := range entries {
		if filepath.Ext(e.Name()) != ".torrent" {
			continue
		}
		p := filepath.Join(dir, e.Name())
		mi, err := metainfo.LoadFromFile(p)
		if err != nil {
			f(IteredUpload{Err: fmt.Errorf("loading metainfo from file %q: %w", p, err)})
			continue
		}
		var umi UploadMetainfo
		// This should really be a new method that assumes to be loading from a file name.
		err = umi.FromTorrentMetainfo(mi, e.Name())
		if err != nil {
			f(IteredUpload{Err: fmt.Errorf("unwrapping upload metainfo from file %q: %w", p, err)})
			continue
		}
		f(IteredUpload{Metainfo: umi, FileInfo: e})
	}
	return nil
}

type UploadOutput struct {
	UploadMetainfo
	AuthToken *string
	Link      *string
}
