package replica

import (
	"errors"

	"github.com/anacrolix/torrent/metainfo"
)

type Info struct {
	TorrentInfo *metainfo.Info
}

func (me *Info) FromTorrentInfo(info *metainfo.Info) error {
	if len(info.Files) != 1 {
		return errors.New("expected single file")
	}
	*me = Info{
		TorrentInfo: info,
	}
	return nil
}

func (me *Info) FilePath() []string {
	return me.TorrentInfo.UpvertedFiles()[0].Path
}

func (me *Info) S3Prefix() S3Prefix {
	return S3Prefix(me.TorrentInfo.Name)
}
