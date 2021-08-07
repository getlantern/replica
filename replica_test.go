package replica

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAppendFileNameUploadUrl(t *testing.T) {
	base, err := NewDynamicEndpoint("https://some.service", nil, nil, false, nil)
	assert.NoError(t, err)
	assert.Equal(t, "https://some.service/upload/lmao", serviceUploadUrl(base, "lmao").String())
	// There's no neat way to handle '/' in the file name, except maybe to accept a variable number
	// of path segments in the handler in the upload service or do the URL path encoding manually.
	// But also having directory separators inside file path components in metainfos will get risky
	// too. So while we handle this, it will just result in a 404 with the current implementation in
	// replica-rust.
	assert.Equal(t, "https://some.service/upload/hello/world", serviceUploadUrl(base, "hello/world").String())
}
