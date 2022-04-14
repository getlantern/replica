package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/anacrolix/missinggo"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/getlantern/golog/testlog"
	"github.com/getlantern/replica/projectpath"
	"github.com/getlantern/replica/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func getDummySearchRequest(t *testing.T) (*http.Request, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	req, err := http.NewRequestWithContext(ctx, "GET", "/search", nil)
	require.NoError(t, err)
	q := req.URL.Query()
	q.Add("s", "cat")
	q.Add("limit", "5")
	req.URL.RawQuery = q.Encode()
	return req, cancel
}

type MockDhtResourceImpl struct {
	path string
}

type fileReaderWithContext struct {
	r *os.File
}

func (r fileReaderWithContext) ReadContext(ctx context.Context, b []byte) (int, error) {
	return r.r.Read(b)
}

func (me MockDhtResourceImpl) Open(ctx context.Context) (missinggo.ReadContexter, bool, error) {
	f, err := os.Open(filepath.Join(me.path))
	if err != nil {
		return nil, false, err
	}
	return fileReaderWithContext{f}, false, nil
}

func (me MockDhtResourceImpl) FetchBep46Payload(context.Context) (metainfo.Hash, error) {
	// Make it return "something". Returning an empty hash will not trigger a download
	return metainfo.Hash{0x41}, nil
}

func (me MockDhtResourceImpl) FetchTorrentFileReader(context.Context, metainfo.Hash) (missinggo.ReadContexter, bool, error) {
	f, err := os.Open(filepath.Join(me.path))
	if err != nil {
		return nil, false, err
	}
	return fileReaderWithContext{f}, false, nil
}

func TestSearch(t *testing.T) {
	dir := t.TempDir()
	input := NewHttpHandlerInput{}
	input.SetDefaults()
	input.RootUploadsDir = dir
	input.ReplicaServiceClient = service.ServiceClient{
		ReplicaServiceEndpoint: func() *url.URL {
			return &url.URL{
				Scheme: "https",
				Host:   "replica-search-aws.lantern.io",
			}
		},
		HttpClient: http.DefaultClient,
	}
	// cacheDir, err := os.UserCacheDir()
	// if err != nil {
	// 	cacheDir = os.TempDir()
	// }
	// cacheDir = filepath.Join(cacheDir, common.DefaultAppName, "dhtup", "data")
	// os.MkdirAll(cacheDir, 0o700)
	localIndexDhtResource := MockDhtResourceImpl{filepath.Join(projectpath.Root, "testdata", "backup-search-index.db")}

	t.Run("Delay backup search roundtripper indefinitely. Primary search roundtripper should be used", func(t *testing.T) {
		input.SetLocalIndex(
			localIndexDhtResource,
			10*time.Minute, // doesn't matter
			t.TempDir(),    // doesn't matter
			func(roundTripperKey string, req *http.Request) error {
				if roundTripperKey == LocalIndexRoundTripperKey {
					log.Debugf("Delaying %s", roundTripperKey)
					time.Sleep(10000 * time.Second)
				}
				return nil
			},
		)
		handler, err := NewHTTPHandler(input)
		require.NoError(t, err)
		defer handler.Close()

		req, cancel := getDummySearchRequest(t)
		defer cancel()
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		require.Equal(t, http.StatusOK, rr.Code)
		require.Equal(t, PrimarySearchRoundTripperKey, rr.Header().Get(roundTripperHeaderKey))
		fmt.Printf("rr.Body.String() = %+v\n", rr.Body.String())
	})

	t.Run("Delay primary search roundtripper indefinitely so that backup search roundtripper is used", func(t *testing.T) {
		input.SetLocalIndex(
			localIndexDhtResource,
			10*time.Minute, // doesn't matter
			t.TempDir(),    // doesn't matter
			func(roundTripperKey string, req *http.Request) error {
				if roundTripperKey == PrimarySearchRoundTripperKey {
					log.Debugf("Delaying %s", roundTripperKey)
					time.Sleep(100000 * time.Second)
				}
				return nil
			},
		)
		handler, err := NewHTTPHandler(input)
		require.NoError(t, err)
		defer handler.Close()

		req, cancel := getDummySearchRequest(t)
		defer cancel()
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		require.Equal(t, http.StatusOK, rr.Code)
		require.Equal(t, LocalIndexRoundTripperKey, rr.Header().Get(roundTripperHeaderKey))
		fmt.Printf("rr.Body.String() = %+v\n", rr.Body.String())
	})

	t.Run("Return failure from primary search roundtripper so that backup search roundtripper is used", func(t *testing.T) {
		input.SetLocalIndex(
			localIndexDhtResource,
			10*time.Minute, // doesn't matter
			t.TempDir(),    // doesn't matter
			func(roundTripperKey string, req *http.Request) error {
				if roundTripperKey == PrimarySearchRoundTripperKey {
					log.Debugf("Delaying %s", roundTripperKey)
					return fmt.Errorf("whatever")
				}
				return nil
			},
		)
		handler, err := NewHTTPHandler(input)
		require.NoError(t, err)
		defer handler.Close()

		req, cancel := getDummySearchRequest(t)
		defer cancel()
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		require.Equal(t, http.StatusOK, rr.Code)
		require.Equal(t, LocalIndexRoundTripperKey, rr.Header().Get(roundTripperHeaderKey))
		fmt.Printf("rr.Body.String() = %+v\n", rr.Body.String())
	})
}

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
	require.NoError(t, err)
	// We expect a token file and metainfo.
	require.Equal(t, 2, len(files))
	assert.Len(t, handler.torrentClient.Torrents(), 0)

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
	require.NoError(t, err)
	// We expect the delete handler to have removed the token and metainfo files.
	require.Empty(t, files)
}

// TestUploadAndDelete_DontSaveUploads makes sure we can upload a file and not
// store the upload data, the metadata, or the admin token locally; and not
// register the upload to the torrent client
func TestUploadAndDelete_DontSaveUploads(t *testing.T) {
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
	input.AddUploadsToTorrentClient = false
	input.StoreUploadsLocally = false
	input.StoreMetainfoFileAndTokenLocally = false
	handler, err := NewHTTPHandler(input)
	require.NoError(t, err)
	defer handler.Close()

	// Assert uploads directory is empty
	uploadsDir := handler.uploadsDir
	files, err := ioutil.ReadDir(uploadsDir)
	require.NoError(t, err)
	require.Empty(t, files)

	// Do the upload
	w := httptest.NewRecorder()
	rw := &NoopInstrumentedResponseWriter{w}
	fileName := "testfile"
	r := httptest.NewRequest("POST", "http://dummy.com/upload?name="+fileName, strings.NewReader("file content"))
	err = handler.handleUpload(rw, r)
	require.NoError(t, err)
	var uploadedObjectInfo objectInfo
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &uploadedObjectInfo))

	// Assert uploads directory is empty
	files, err = ioutil.ReadDir(uploadsDir)
	require.NoError(t, err)
	require.Empty(t, files)
	require.Empty(t, handler.torrentClient.Torrents())
}
