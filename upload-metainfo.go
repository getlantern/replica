package replica

import (
	"errors"
	"fmt"

	"github.com/anacrolix/torrent/metainfo"
	"github.com/google/uuid"
)

// Wraps an upload metainfo.Metainfo
type UploadMetainfo struct {
	*metainfo.MetaInfo
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
	if mi.Comment == "Replica" {
		u, err := uuid.Parse(info.Name)
		if err != nil {
			return fmt.Errorf("parsing uuid from info name: %w", err)
		}
		me.Upload = Upload{
			UploadPrefix: UploadPrefix{u},
			Endpoint:     DefaultEndpoint,
		}
		return nil
	}
	return me.Upload.FromExactSource(mi.Comment)
}

func (me UploadMetainfo) FilePath() []string {
	return me.Info.UpvertedFiles()[0].Path
}
