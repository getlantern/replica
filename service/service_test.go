package service

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAppendFileNameUploadUrl(t *testing.T) {
	fetchBaseUrlFunc := func() *url.URL {
		u, err := url.Parse("https://some.service")
		require.NoError(t, err)
		return u
	}
	require.Equal(t,
		"https://some.service/upload/lmao",
		serviceUploadUrl(fetchBaseUrlFunc, "lmao").String())
	// There's no neat way to handle '/' in the file name, except maybe to
	// accept a variable number of path segments in the handler in the upload
	// service or do the URL path encoding manually.  But also having directory
	// separators inside file path components in metainfos will get risky too.
	// So while we handle this, it will just result in a 404 with the current
	// implementation in replica-rust.
	require.Equal(t,
		"https://some.service/upload/hello/world",
		serviceUploadUrl(fetchBaseUrlFunc, "hello/world").String())
}
