package server

import (
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/getlantern/flashlight/common"
	"github.com/getlantern/flashlight/testutils"
	"github.com/getlantern/golog/testlog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProxy(t *testing.T) {
	stopCapture := testlog.Capture(t)
	defer stopCapture()

	uc := common.NewUserConfigData("device", 0, "token", nil, "en-US")

	m := &testutils.MockRoundTripper{Header: http.Header{}, Body: strings.NewReader("GOOD")}
	httpClient = &http.Client{Transport: m}

	l, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)
	addr := l.Addr()
	url_ := url.URL{
		Scheme: "http",
		Host:   addr.String(),
		Path:   "/replica/search",
	}
	t.Logf("Test server listening at %v", url_)

	handler := proxyHandler(
		uc,
		func() *url.URL {
			u, err := url.Parse("http://" + addr.String())
			require.NoError(t, err)
			return u
		},
		func(*http.Response) error {
			return nil
		},
	)
	go http.Serve(l, handler)

	{
		req, err := http.NewRequest("OPTIONS", url_.String(), nil)
		require.NoError(t, err)
		req.Header.Set("Origin", "a.com")
		resp, err := (&http.Client{}).Do(req)
		if assert.NoError(t, err, "OPTIONS request should succeed") {
			assert.Equal(t, 200, resp.StatusCode, "should respond 200 to OPTIONS")
			assert.Equal(t, "GET", resp.Header.Get("Access-Control-Allow-Methods"), "should respond with correct CORS method header")
			_ = resp.Body.Close()
		}
		require.Nil(t, m.Req, "should not pass the OPTIONS request to origin server")
	}

	{
		req, err := http.NewRequest("GET", url_.String(), nil)
		require.NoError(t, err)
		req.Header.Set("Origin", "a.com")
		resp, err := (&http.Client{}).Do(req)
		if assert.NoError(t, err, "GET request should succeed") {
			require.Equal(t, 200, resp.StatusCode, "should respond 200 to GET")
			_ = resp.Body.Close()
		}
		if assert.NotNil(t, m.Req, "should pass through non-OPTIONS requests to origin server") {
			t.Log(m.Req)
			require.Equal(t, "device", m.Req.Header.Get("x-lantern-device-id"), "should include device id header in request to search api")
			require.Equal(t, "token", m.Req.Header.Get("x-lantern-pro-token"), "should include pro token header in request to search api")
		}
	}
}
