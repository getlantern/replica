package replica

import (
	"net/url"
	"testing"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/stretchr/testify/require"
)

func TestCreateLink(t *testing.T) {
	const infoHashHex = "deadbeefc0ffeec0ffeedeadbeefc0ffeec0ffee"
	var infoHash torrent.InfoHash
	require.NoError(t, infoHash.FromHexString(infoHashHex))
	link := CreateLink(infoHash, "big long uuid/herp.txt", []string{"nice name"})
	require.EqualValues(t,
		"magnet:?xt=urn:btih:deadbeefc0ffeec0ffeedeadbeefc0ffeec0ffee"+
			"&as=https%3A%2F%2Fgetlantern-replica.s3-ap-southeast-1.amazonaws.com%2Fbig+long+uuid%2Fherp.txt%2Ftorrent"+
			"&dn=nice+name"+
			"&so=0"+ // Not sure if we can rely on the ordering of params, hope so.
			"&ws=https%3A%2F%2Fgetlantern-replica.s3-ap-southeast-1.amazonaws.com%2Fbig+long+uuid%2Fherp.txt%2Fdata%2F"+
			"&xs=replica%3Abig+long+uuid%2Fherp.txt", link)
}

// This is to check that s3KeyFromMagnet doesn't return an error if there's no replica xs parameter.
// This is valid for Replica magnet links that don't refer to items on S3.
func TestS3PrefixFromMagnetMissingXs(t *testing.T) {
	m, err := metainfo.ParseMagnetURI("magnet:?xt=urn:btih:b84d0051d6cc64eb48bf8c47dd44320f69c17544&dn=Test+Drive+Unlimited+ReincarnaTion%2FTest+Drive+Unlimited+ReincarnaTion.exe&so=0")
	require.NoError(t, err)
	s3Key, err := S3PrefixFromMagnet(m)
	require.NoError(t, err)
	require.EqualValues(t, "", s3Key)
}

// This is to check that s3KeyFromMagnet doesn't return an error if there's no replica xs parameter.
// This is valid for Replica magnet links that don't refer to items on S3.
func TestS3KeyFromReplicaMagnetOpaqueKey(t *testing.T) {
	m, err := metainfo.ParseMagnetURI("magnet:?xt=urn:btih:bee25d279cb0ac33b13ec6c35ab5128e8a0279f6&as=https%3A%2F%2Fgetlantern-replica.s3-ap-southeast-1.amazonaws.com%2F4cfacbd0-811c-4319-9d57-87c484c14814%2Fhornady.pdf&dn=hornady.pdf&tr=http%3A%2F%2Fs3-tracker.ap-southeast-1.amazonaws.com%3A6969%2Fannounce&xs=replica%3A4cfacbd0-811c-4319-9d57-87c484c14814%2Fhornady.pdf&so=0")
	require.NoError(t, err)
	u, _ := url.Parse(m.Params.Get("xs"))
	t.Logf("%#v", u)
	s3Key, err := S3PrefixFromMagnet(m)
	require.NoError(t, err)
	require.EqualValues(t, "4cfacbd0-811c-4319-9d57-87c484c14814/hornady.pdf", s3Key)
}
