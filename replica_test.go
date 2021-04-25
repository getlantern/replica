package replica

import (
	"net/url"
	"testing"

	qt "github.com/frankban/quicktest"
)

func TestAppendFileNameUploadUrl(t *testing.T) {
	c := qt.New(t)
	base, err := url.Parse("https://some.service")
	c.Assert(err, qt.IsNil)
	c.Check(serviceUploadUrl(base, "lmao").String(), qt.Equals, "https://some.service/upload/lmao")
	// There's no neat way to handle '/' in the file name, except maybe to accept a variable number
	// of path segments in the handler in the upload service or do the URL path encoding manually.
	// But also having directory separators inside file path components in metainfos will get risky
	// too. So while we handle this, it will just result in a 404 with the current implementation in
	// replica-rust.
	c.Check(serviceUploadUrl(base, "hello/world").String(), qt.Equals, "https://some.service/upload/hello/world")
}
