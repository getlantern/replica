package replica

import (
	"net/url"
	"strings"
	"testing"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/stretchr/testify/require"

	"github.com/getlantern/replica/service"
)

func TestCreateLink(t *testing.T) {
	const infoHashHex = "deadbeefc0ffeec0ffeedeadbeefc0ffeec0ffee"
	var infoHash torrent.InfoHash
	require.NoError(t, infoHash.FromHexString(infoHashHex))
	upload := service.NewUuidPrefix()
	link := CreateLink(infoHash, upload, []string{"nice name"})
	uuidString := upload.String()
	require.EqualValues(t,
		[]string{
			"magnet:?xt=urn:btih:deadbeefc0ffeec0ffeedeadbeefc0ffeec0ffee",
			"dn=nice+name",
			"so=0", // Not sure if we can rely on the ordering of params, hope so.
			"xs=replica%3A" + uuidString,
		}, strings.Split(link, "&"))
}

// This is to check that s3KeyFromMagnet doesn't return an error if there's no replica xs parameter.
// This is valid for Replica magnet links that don't refer to items on S3.
func TestS3PrefixFromMagnetMissingXs(t *testing.T) {
	m, err := metainfo.ParseMagnetURI("magnet:?xt=urn:btih:b84d0051d6cc64eb48bf8c47dd44320f69c17544&dn=Test+Drive+Unlimited+ReincarnaTion%2FTest+Drive+Unlimited+ReincarnaTion.exe&so=0")
	require.NoError(t, err)
	err = new(service.Upload).FromMagnet(m)
	require.Error(t, err)
}

// This is to check that s3KeyFromMagnet doesn't return an error if there's no replica xs parameter.
// This is valid for Replica magnet links that don't refer to items on S3.
func TestS3KeyFromReplicaMagnetOpaqueKey(t *testing.T) {
	m, err := metainfo.ParseMagnetURI("magnet:?xt=urn:btih:bee25d279cb0ac33b13ec6c35ab5128e8a0279f6&as=https%3A%2F%2Fs3.ap-southeast-1.amazonaws.com%2Fgetlantern-replica%2F4cfacbd0-811c-4319-9d57-87c484c14814%2Fhornady.pdf&dn=hornady.pdf&tr=http%3A%2F%2Fs3-tracker.ap-southeast-1.amazonaws.com%3A6969%2Fannounce&xs=replica%3A4cfacbd0-811c-4319-9d57-87c484c14814&so=0")
	require.NoError(t, err)
	u, _ := url.Parse(m.Params.Get("xs"))
	t.Logf("%#v", u)
	var s3Key service.Upload
	err = s3Key.FromMagnet(m)
	require.NoError(t, err)
	require.EqualValues(t, "4cfacbd0-811c-4319-9d57-87c484c14814", s3Key.String())
}
