package server

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/getlantern/golog/testlog"
	"github.com/getlantern/replica/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUploadAndDelete makes sure we can upload and then subsequently delete a given file.
func TestUploadAndDelete(t *testing.T) {
	stopCapture := testlog.Capture(t)
	defer stopCapture()

	dir := t.TempDir()
	input := NewHttpHandlerInput{}
	input.SetDefaults()
	input.ReplicaServiceClient = service.ServiceClient{
		// XXX Use this endpoint to communicate directly with replica-rust,
		ReplicaServiceEndpoint: func() *url.URL { return service.GlobalChinaDefaultServiceUrl },
		// For local tests, build & run replica-rust and use the local loopback address
		// ReplicaServiceEndpoint: func () *url.URL { return &url.URL{Scheme: "http", Host: "localhost:8080"} }
		HttpClient: http.DefaultClient,
	}
	input.RootUploadsDir = dir
	handler, err := NewHTTPHandler(input)
	require.NoError(t, err)
	defer handler.Close()

	uploadsDir := handler.uploadsDir
	files, err := ioutil.ReadDir(uploadsDir)
	assert.NoError(t, err)
	assert.Empty(t, files)

	w := httptest.NewRecorder()
	rw := &NoopInstrumentedResponseWriter{w}
	fileName := "testfile"
	r := httptest.NewRequest("POST", "http://dummy.com/upload?name="+fileName, strings.NewReader("file content"))
	err = handler.handleUpload(rw, r)
	require.NoError(t, err)

	var uploadedObjectInfo objectInfo
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &uploadedObjectInfo))

	files, err = ioutil.ReadDir(uploadsDir)
	assert.NoError(t, err)
	// We expect a token file and metainfo.
	assert.Equal(t, 2, len(files))

	magnetLink := uploadedObjectInfo.Link

	// We're bypassing the actual HTTP server route handling, so the domain and path can be anything
	// here.
	url_ := (&url.URL{
		Scheme:   "http",
		Host:     "dummy.com",
		Path:     "/upload",
		RawQuery: url.Values{"link": {magnetLink}}.Encode(),
	}).String()
	log.Debugf("delete url: %q", url_)

	d := httptest.NewRequest("GET", url_, nil)
	err = handler.handleDelete(rw, d)
	require.NoError(t, err)

	files, err = ioutil.ReadDir(uploadsDir)
	assert.NoError(t, err)
	// We expect the delete handler to have removed the token and metainfo files.
	assert.Empty(t, files)
}

func TestClearUploadsHandler(t *testing.T) {
	stopCapture := testlog.Capture(t)
	defer stopCapture()

	// Initialize server and assert success
	dir := t.TempDir()
	input := NewHttpHandlerInput{}
	input.SetDefaults()
	input.ReplicaServiceClient = service.ServiceClient{
		// XXX Use this endpoint to communicate directly with replica-rust,
		ReplicaServiceEndpoint: func() *url.URL { return service.GlobalChinaDefaultServiceUrl },
		// For local tests, build & run replica-rust and use the local loopback address
		// ReplicaServiceEndpoint: func () *url.URL { return &url.URL{Scheme: "http", Host: "localhost:8080"} }
		HttpClient: http.DefaultClient,
	}
	input.RootUploadsDir = dir
	handler, err := NewHTTPHandler(input)
	require.NoError(t, err)
	require.DirExists(t, handler.uploadsDir)
	defer handler.Close()

	// Make an upload
	w := httptest.NewRecorder()
	rw := &NoopInstrumentedResponseWriter{w}
	fileName := "testfile"
	r := httptest.NewRequest("POST", "http://dummy.com/upload?name="+fileName, strings.NewReader("file content"))
	err = handler.handleUpload(rw, r)
	require.NoError(t, err)

	// Assert it succeeded
	var uploadedObjectInfo objectInfo
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &uploadedObjectInfo))
	files, err := ioutil.ReadDir(handler.uploadsDir)
	require.NoError(t, err)
	// We expect a token file and metainfo.
	require.Equal(t, 2, len(files))

	// Clear uploads dir and assert success
	err = handler.handleClearUploads(rw,
		httptest.NewRequest("POST", "http://dummy.com/clear_uploads", nil))
	require.NoError(t, err)
	require.DirExists(t, handler.uploadsDir)
	files, err = ioutil.ReadDir(handler.uploadsDir)
	require.NoError(t, err)
	// We expect a token file and metainfo.
	require.Empty(t, files)
}
