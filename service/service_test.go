package service

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAppendFileNameUploadUrl(t *testing.T) {
	fetchBaseUrlFunc := func() *url.URL {
		u, err := url.Parse("https://some.service")
		require.NoError(t, err)
		return u
	}
	assert.Equal(t,
		"https://some.service/upload/lmao",
		serviceUploadUrl(fetchBaseUrlFunc, "lmao").String())
	// There's no neat way to handle '/' in the file name, except maybe to
	// accept a variable number of path segments in the handler in the upload
	// service or do the URL path encoding manually.  But also having directory
	// separators inside file path components in metainfos will get risky too.
	// So while we handle this, it will just result in a 404 with the current
	// implementation in replica-rust.
	assert.Equal(t,
		"https://some.service/upload/hello/world",
		serviceUploadUrl(fetchBaseUrlFunc, "hello/world").String())

	// Check that non-ASCII file names are encoded for upload, and decode back in the same manner
	// that replica-rust works. https://github.com/getlantern/lantern-internal/issues/5401
	cyrillicUploadUrl := serviceUploadUrl(fetchBaseUrlFunc, "rf200_now-Подписаться__Бот_для_поиска_своих__Резервный_канал.mov")
	assert.Equal(t,
		"https://some.service/upload/rf200_now-%D0%9F%D0%BE%D0%B4%D0%BF%D0%B8%D1%81%D0%B0%D1%82%D1%8C%D1%81%D1%8F__%D0%91%D0%BE%D1%82_%D0%B4%D0%BB%D1%8F_%D0%BF%D0%BE%D0%B8%D1%81%D0%BA%D0%B0_%D1%81%D0%B2%D0%BE%D0%B8%D1%85__%D0%A0%D0%B5%D0%B7%D0%B5%D1%80%D0%B2%D0%BD%D1%8B%D0%B9_%D0%BA%D0%B0%D0%BD%D0%B0%D0%BB.mov",
		cyrillicUploadUrl.String())
	assert.Equal(t, "/upload/rf200_now-Подписаться__Бот_для_поиска_своих__Резервный_канал.mov", cyrillicUploadUrl.Path)
}
