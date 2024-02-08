package server

import (
	"bytes"
	"context"
	"encoding/json"
	stdErrors "errors"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/anacrolix/chansync"
	"github.com/anacrolix/confluence/confluence"
	"github.com/anacrolix/dht/v2"
	analog "github.com/anacrolix/log"
	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/anacrolix/torrent/storage"
	sqliteStorage "github.com/anacrolix/torrent/storage/sqlite"
	"github.com/getlantern/dhtup"
	"github.com/getlantern/errors"
	"github.com/getlantern/golog"
	metascrubber "github.com/getlantern/meta-scrubber"
	"github.com/getlantern/ops"
	"github.com/getsentry/sentry-go"
	"github.com/gorilla/mux"
	"github.com/kennygrant/sanitize"

	"github.com/getlantern/replica/service"
)

const handlerLogPrefix = "replica-pkg"

var log = golog.LoggerFor(handlerLogPrefix)

type HttpHandler struct {
	// Used to handle non-Replica specific routes. (Some of the hard work has been done!). This will
	// probably go away soon, as I pick out the parts we actually need.
	confluence    confluence.Handler
	torrentClient *torrent.Client
	// Where to store torrent client data.
	dataDir     string
	uploadsDir  string
	router      *mux.Router
	searchProxy http.Handler
	NewHttpHandlerInput
	uploadStorage  storage.ClientImplCloser
	defaultStorage storage.ClientImplCloser
	closed         chansync.SetOnce
}

func getMetainfoUrls(ro ReplicaOptions, prefix string) (ret []string) {
	for _, s := range ro.GetWebseedBaseUrls() {
		ret = append(ret, fmt.Sprintf("%s%s/torrent", s, prefix))
	}
	return
}

func getWebseedUrls(ro ReplicaOptions, prefix string) (ret []string) {
	for _, s := range ro.GetWebseedBaseUrls() {
		ret = append(ret, fmt.Sprintf("%s%s/data/", s, prefix))
	}
	return
}

type NewHttpHandlerInput struct {
	// Should be used for webseeding and torrent sources.
	TorrentWebTransport http.RoundTripper
	// Used to proxy http calls
	TorrentClientHTTPProxy func(*http.Request) (*url.URL, error)
	// Takes a tracker's hostname and requests DNS A and AAAA records.
	// Used for proxying DNS lookups through https (i.e., dns-over-https)
	TorrentClientLookupTrackerIp func(*url.URL) ([]net.IP, error)
	// Root directory of the torrent uploads.
	// This is usually postfixed with "$RootUploadsDir/replica/uploads"
	RootUploadsDir string
	// Location of torrent client data
	// If left empty, will use os.UserCacheDir() or os.TempDir() if the former
	// is inaccessible
	CacheDir string
	// The Application Name, used for generating cache directories specific to this application
	AppName string
	// http.Client with a proxying RoundTripper that simultaneously utilizes
	// domain-fronting and proxies for GET and HEAD requests, and that
	// sequentially uses proxies and then domain-fronting for other types of
	// requests.
	HttpClient *http.Client
	// For uploads, deletes, and other behaviour serviced by replica-rust using an API in the
	// replica repo.
	ReplicaServiceClient service.ServiceClient
	// Doing this might be a privacy concern, since users could be singled out for being the
	// first/only uploader for content.
	AddUploadsToTorrentClient bool
	// Retain a copy of upload file data. This would save downloading our own uploaded content if we
	// intend to seed it.
	StoreUploadsLocally bool
	// Create a metainfo file and admin token
	// Admin token is used if the uploader wants to delete their file later on
	StoreMetainfoFileAndTokenLocally bool
	OnRequestReceived                func(handler string, extraInfo string)
	// Indirection for fetching the latest replica options from the global config. Must not return nil. Probably
	// shouldn't be nil itself either (at least if any code that interacts with Replica S3 and centralized BitTorrent
	// infrastructure is to be used).
	GlobalConfig func() ReplicaOptions
	// This sets the DHT node as 'read-only' as well as disable seeding in the
	// torrent client
	ReadOnlyNode bool
	// Callback for adding common headers to outbound requests
	AddCommonHeaders func(*http.Request)
	// ProcessCORSHeaders processes CORS requests on localhost.
	// It returns true if the request is a valid CORS request
	// from an allowed origin and false otherwise.
	ProcessCORSHeaders func(responseHeaders http.Header, r *http.Request) bool
	// Instruments the given ResponseWriter for tracking metrics under the given label
	InstrumentResponseWriter func(w http.ResponseWriter, label string) InstrumentedResponseWriter

	// File path to a backup sqlite search index.
	// If this is not nil, search requests would use this local index as a
	// backup in case the remote replica-rust search index is unreachable.
	//
	// Use it by running NewHttpHandlerInput.SetLocalIndex().
	dhtContext              dhtup.Context
	LocalIndexDhtDownloader *LocalIndexDhtDownloader
	// Intercepts a Replica search request. If returned error is not-nil, the
	// roundtripper will return that error. Used only for testing
	DualSearchIndexRoundTripperInterceptRequestFunc func(string, *http.Request) error
}

// Returns candidate cache directories in order of preference.
func candidateCacheDirs(appName, inputCacheDir string) (ret []string) {
	if inputCacheDir != "" {
		ret = append(ret, inputCacheDir)
	}
	osUserCacheDir, err := os.UserCacheDir()
	if err == nil {
		ret = append(ret, osUserCacheDir)
	} else {
		log.Debugf("Ignorable error while getting the user cache dir: %v", err)
	}
	ret = append(ret, os.TempDir())
	for i := range ret {
		ret[i] = filepath.Join(ret[i], appName, "replica")
	}
	return
}

// Tries to create a replica cache directory with appropriate permissions, returning the best
// candidate if all attempts fail.
func prepareCacheDir(appName, inputCacheDir string) string {
	candidates := candidateCacheDirs(appName, inputCacheDir)
	for _, dir := range candidates {
		err := os.MkdirAll(dir, 0o700)
		if err == nil {
			return dir
		}
		log.Errorf("creating candidate replica cache dir %q: %v", dir, err)
	}
	return candidates[0]
}

// NewHTTPHandler creates a new http.Handler for calls to replica.
func NewHTTPHandler(
	input NewHttpHandlerInput,
) (_ *HttpHandler, err error) {
	replicaCacheDir := prepareCacheDir(input.AppName, input.CacheDir)
	log.Debugf("replica cache dir: %q", replicaCacheDir)
	uploadsDir := filepath.Join(input.RootUploadsDir, "replica", "uploads")
	err = os.MkdirAll(uploadsDir, 0o700)
	if err != nil {
		return nil, errors.New("mkdir uploadsDir %v: %v", uploadsDir, err)
	}
	replicaDataDir := filepath.Join(replicaCacheDir, "data")
	err = os.MkdirAll(replicaDataDir, 0o700)
	if err != nil {
		return nil, errors.New("mkdir replicaDataDir %v: %v", replicaDataDir, err)
	}
	cfg := torrent.NewDefaultClientConfig()
	cfg.DisableIPv6 = true
	cfg.HTTPProxy = input.TorrentClientHTTPProxy
	cfg.LookupTrackerIp = input.TorrentClientLookupTrackerIp
	// This should not be used, we're specifying our own storage for uploads and general views
	// respectively.
	cfg.DataDir = "\x00"
	// https://github.com/getlantern/android-lantern/issues/533
	if input.ReadOnlyNode {
		cfg.Seed = false
		cfg.ConfigureAnacrolixDhtServer = func(dhtCfg *dht.ServerConfig) {
			dhtCfg.Passive = true
		}
	} else {
		cfg.Seed = true
	}
	cfg.HeaderObfuscationPolicy.Preferred = true
	cfg.HeaderObfuscationPolicy.RequirePreferred = true
	// cfg.Debug = true
	cfg.Logger = analog.Default.WithContextText(handlerLogPrefix + ".torrent-client")

	var opts sqliteStorage.NewDirectStorageOpts
	opts.Path = filepath.Join(replicaCacheDir, "storage-cache.db")
	os.Remove(opts.Path)
	opts.Capacity = 5 << 30
	defaultStorage, err := sqliteStorage.NewDirectStorage(opts)
	if err != nil {
		return nil, errors.New("creating torrent storage cache with opts %+v: %v", opts, err)
	}
	defer func() {
		if err != nil {
			defaultStorage.Close()
		}
	}()
	cfg.DefaultStorage = defaultStorage

	cfg.Callbacks.ReceivedUsefulData = append(cfg.Callbacks.ReceivedUsefulData,
		func(event torrent.ReceivedUsefulDataEvent) {
			op := ops.Begin("replica_torrent_peer_sent_data")
			op.Set("remote_addr", event.Peer.RemoteAddr.String())
			op.Set("remote_network", event.Peer.Network)
			op.Set("useful_bytes_count", float64(len(event.Message.Piece)))
			op.Set("info_hash", event.Peer.Torrent().InfoHash().HexString())
			op.End()
			log.Tracef("reported %v bytes from %v over %v",
				len(event.Message.Piece),
				event.Peer.RemoteAddr.String(),
				event.Peer.Network)
		})
	cfg.WebTransport = input.TorrentWebTransport
	torrentClient, err := torrent.NewClient(cfg)
	if err != nil {
		log.Errorf("Error creating client: %v", err)
		if torrentClient != nil {
			torrentClient.Close()
		}
		// Try an ephemeral port in case there was an error binding.
		cfg.ListenPort = 0
		torrentClient, err = torrent.NewClient(cfg)
		if err != nil {
			return nil, errors.New("starting torrent client: %v", err)
		}
	}

	handler := &HttpHandler{
		confluence: confluence.Handler{
			TC: torrentClient,
			MetainfoCacheDir: func() *string {
				s := filepath.Join(replicaCacheDir, "metainfos")
				return &s
			}(),
		},
		torrentClient: torrentClient,
		dataDir:       replicaDataDir,
		uploadsDir:    uploadsDir,
		router:        mux.NewRouter(),
		searchProxy: http.StripPrefix("/search", proxyHandler(
			input,
			nil)),
		// I think the standard file-storage implementation is sufficient here because we guarantee
		// unique info name/prefixes for uploads (which the default file implementation does not).
		// There's another implementation that injects the infohash as a prefix to ensure uniqueness
		// of final file names.
		uploadStorage:       storage.NewFile(replicaDataDir),
		defaultStorage:      defaultStorage,
		NewHttpHandlerInput: input,
	}

	// XXX <03-02-22, soltzen> See
	// https://github.com/getlantern/lantern-internal/issues/5226 for more
	// context.
	// Requests coming from lantern-desktop's UI client
	// will always carry lantern-desktop's server address (i.e.,
	// [here](https://github.com/getlantern/lantern-desktop/blob/87370cca9c895d0e0296b4d16e292ad8adbdae33/server/defaults_static.go#L1))
	// in their 'Host' header (like this: 'Host: localhost:16823'). This is
	// problamatic for Replica servers. So, best to either wipe it or assign it
	// as the URL's host
	//
	// This middleware runs before all routes
	prepareRequestMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Host = r.URL.Host
			next.ServeHTTP(w, r)
		})
	}
	handler.router.Use(prepareRequestMiddleware)
	handler.router.HandleFunc("/heartbeat", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler.router.HandleFunc("/search", handler.wrapHandlerError("replica_search", handler.handleSearch))
	handler.router.HandleFunc("/search/serp_web", handler.wrapHandlerError("replica_search", handler.handleSearch))
	handler.router.HandleFunc("/search/news", handler.wrapHandlerError("replica_search", handler.handleSearch))
	handler.router.HandleFunc("/thumbnail", handler.wrapHandlerError("replica_thumbnail", handler.handleMetadata("thumbnail")))
	handler.router.HandleFunc("/duration", handler.wrapHandlerError("replica_duration", handler.handleMetadata("duration")))
	handler.router.HandleFunc("/upload", handler.wrapHandlerError("replica_upload", handler.handleUpload))
	handler.router.HandleFunc("/uploads", handler.wrapHandlerError("replica_uploads", handler.handleUploads))
	handler.router.HandleFunc("/view", handler.wrapHandlerError("replica_view", handler.handleView))
	handler.router.HandleFunc("/download", handler.wrapHandlerError("replica_view", handler.handleDownload))
	handler.router.HandleFunc("/delete", handler.wrapHandlerError("replica_delete", handler.handleDelete))
	handler.router.HandleFunc("/object_info", handler.wrapHandlerError("replica_object_info", handler.handleObjectInfo))
	handler.router.HandleFunc("/debug/dht", func(w http.ResponseWriter, r *http.Request) {
		for _, ds := range torrentClient.DhtServers() {
			ds.WriteStatus(w)
		}
	})
	// TODO(anacrolix): Actually not much of Confluence is used now, probably none of the routes, so
	// this might go away soon.
	// Confluence embeds its own routes, so make sure to pass the path and request in its entirety.
	handler.router.PathPrefix("/").Handler(&handler.confluence)

	if input.AddUploadsToTorrentClient {
		if err := service.IterUploads(uploadsDir, func(iu service.IteredUpload) {
			if iu.Err != nil {
				log.Errorf("error while iterating uploads: %v", iu.Err)
				return
			}
			err := handler.addUploadTorrent(iu.Metainfo.MetaInfo, true)
			if err != nil {
				log.Errorf("error adding existing upload from %q to torrent client: %v", iu.FileInfo.Name(), err)
			} else {
				log.Debugf("added previous upload %q to torrent client from file %q", iu.Metainfo.Upload, iu.FileInfo.Name())
			}
		}); err != nil {
			handler.Close()
			return nil, errors.New("iterating through uploads: %v", err)
		}
	}
	go handler.metricsExporter()
	return handler, nil
}

func (me *HttpHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Debugf("replica server request path: %q", r.URL.Path)
	me.ProcessCORSHeaders(w.Header(), r)
	me.router.ServeHTTP(w, r)
}

func (me *HttpHandler) Close() {
	me.torrentClient.Close()
	me.uploadStorage.Close()
	me.defaultStorage.Close()
	me.closed.Set()
}

// handlerError is just a small wrapper around errors so that we can more easily return them from
// handlers and then inspect them in our handler wrapper
type handlerError struct {
	statusCode int
	error
}

// encoderError is a small wrapper around error so we know that this is an error
// during encoding/writing a response and we can avoid trying to re-write headers
type encoderWriterError struct {
	error
}

func (me *HttpHandler) wrapHandlerError(
	opName string,
	handler func(InstrumentedResponseWriter, *http.Request) error,
) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		w := me.InstrumentResponseWriter(rw, opName)
		defer w.Finish()
		if err := handler(w, r); err != nil {
			log.Errorf("in %q handler: %v", opName, err)
			w.FailIf(err)

			// we may want to only ultimately only report server errors here (ie >=500)
			// but since we're also the client I think this makes some sense for now
			sentry.ConfigureScope(func(scope *sentry.Scope) {
				scope.SetLevel(func() sentry.Level {
					if r.Context().Err() == context.Canceled && stdErrors.Is(err, context.Canceled) {
						return sentry.LevelInfo
					}
					return sentry.LevelError
				}())
			})
			sentry.CaptureException(err)

			// if it's an error during encoding+writing, don't attempt to
			// write new headers and body
			if e, ok := err.(encoderWriterError); ok {
				log.Errorf("error writing json: %v", e)
				return
			}

			var statusCode int
			if e, ok := err.(handlerError); ok {
				statusCode = e.statusCode
			} else {
				statusCode = http.StatusInternalServerError
			}

			resp := map[string]interface{}{
				"statusCode": statusCode,
				"error":      err.Error(),
			}

			var writingEncodingErr error
			writingEncodingErr = encodeJsonErrorResponse(rw, resp, statusCode)

			if writingEncodingErr != nil {
				log.Errorf("error writing json error response: %v", writingEncodingErr)
			}
		}
	}
}

func (me *HttpHandler) writeNewUploadAuthTokenFile(auth string, prefix service.Prefix) error {
	tokenFilePath := me.uploadTokenPath(prefix)
	f, err := os.OpenFile(
		tokenFilePath,
		os.O_WRONLY|os.O_CREATE|os.O_EXCL|os.O_TRUNC,
		0o440,
	)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(auth)
	if err != nil {
		return err
	}
	log.Debugf("wrote %q to %q", auth, tokenFilePath)
	return f.Close()
}

type CountWriter struct {
	BytesWritten int64
}

func (me *CountWriter) Write(b []byte) (int, error) {
	me.BytesWritten += int64(len(b))
	return len(b), nil
}

func (me *HttpHandler) handleUpload(rw InstrumentedResponseWriter, r *http.Request) error {
	// Set status code to 204 to handle preflight CORS check
	if r.Method == "OPTIONS" {
		rw.WriteHeader(http.StatusNoContent)
		return nil
	}

	switch r.Method {
	default:
		return handlerError{
			http.StatusMethodNotAllowed,
			errors.New("expected method supporting request body"),
		}
	case http.MethodPost, http.MethodPut:
		// Maybe we can differentiate form handling based on the method.
	}

	uploadOptions := service.NewUploadOptions(r)
	fileName := r.URL.Query().Get("name")

	var fileReader io.Reader
	{
		// There are streaming ways and helpers for temporary files for this if size becomes an issue.
		formFile, fileHeader, err := r.FormFile("file")
		if err == nil {
			fileReader = formFile
			if fileName == "" {
				fileName = fileHeader.Filename
			}
		} else {
			log.Debugf("error getting upload file as form file: %v", err)
			fileReader = r.Body
		}
	}

	scrubbedReader, err := metascrubber.GetScrubber(fileReader)
	if err != nil {
		return errors.New("getting metascrubber: %v", err)
	}

	var cw CountWriter
	replicaUploadReader := io.TeeReader(scrubbedReader, &cw)

	var (
		tmpFile    *os.File
		tmpFileErr error
	)
	if me.StoreUploadsLocally {
		tmpFile, tmpFileErr = func() (*os.File, error) {
			// This is for testing temp file failures.
			const forceTempFileFailure = false
			if forceTempFileFailure {
				return nil, errors.New("sike")
			}
			return ioutil.TempFile("", "")
		}()
		if tmpFileErr == nil {
			defer os.Remove(tmpFile.Name())
			defer tmpFile.Close()
			replicaUploadReader = io.TeeReader(replicaUploadReader, tmpFile)
		} else {
			// This isn't good, but as long as we can add the torrent file metainfo to the local
			// client, we can still spread the metadata, and S3 can take care of the data.
			log.Errorf("error creating temporary file: %v", tmpFileErr)
		}
	}

	output, err := me.ReplicaServiceClient.Upload(replicaUploadReader, fileName, uploadOptions)
	// me.GaSession.EventWithLabel("replica", "upload", path.Ext(fileName))
	if me.OnRequestReceived != nil {
		me.OnRequestReceived("upload", path.Ext(fileName))
	}
	log.Debugf("uploaded %d bytes", cw.BytesWritten)
	if err != nil {
		return errors.New("uploading with replica client: %v", err)
	}
	upload := output.Upload
	log.Debugf("uploaded replica key %q", upload)
	rw.Set("upload_s3_key", upload.PrefixString())

	if me.StoreMetainfoFileAndTokenLocally {
		var metainfoBytes bytes.Buffer
		err = output.MetaInfo.Write(&metainfoBytes)
		if err != nil {
			return errors.New("writing metainfo: %v", err)
		}
		err = storeUploadedTorrent(&metainfoBytes, me.uploadMetainfoPath(upload))
		if err != nil {
			return errors.New("storing uploaded torrent: %v", err)
		}

		if err = me.writeNewUploadAuthTokenFile(*output.AuthToken, upload.Prefix); err != nil {
			log.Errorf("error writing upload auth token file: %v", err)
		}
	}

	if tmpFileErr == nil && me.StoreUploadsLocally {
		// Windoze might complain if we don't close the handle before moving the file, plus it's
		// considered good practice to check for close errors after writing to a file. (I'm not
		// closing it, but at least I'm flushing anything, if it's incomplete at this point, the
		// torrent client will complete it as required.
		tmpFile.Close()
		// Move the temporary file, which contains the upload body, to the data directory for the
		// torrent client, in the location it expects.
		dst := filepath.Join(append([]string{me.dataDir, upload.String()}, output.Info.UpvertedFiles()[0].Path...)...)
		err = os.MkdirAll(filepath.Dir(dst), 0o700)
		if err != nil {
			return errors.New("creating data directory: %v: %v", dst, err)
		}
		err = os.Rename(tmpFile.Name(), dst)
		if err != nil {
			// Not fatal: See above, we only really need the metainfo to be added to the torrent.
			log.Errorf("error renaming file: %v", err)
		}
	}
	if me.AddUploadsToTorrentClient {
		err = me.addUploadTorrent(output.MetaInfo, true)
		if err != nil {
			return errors.New("adding torrent: %v", err)
		}
	}
	var oi objectInfo
	err = oi.FromUploadMetainfo(output.UploadMetainfo, time.Now())
	if err != nil {
		return errors.New("getting objectInfo from upload metainfo: %v", err)
	}
	// We can clobber with what should be a superior link directly from the upload service endpoint.
	if output.Link != nil {
		oi.Link = *output.Link
	}
	return encodeJsonResponse(rw, oi)
}

func (me *HttpHandler) addUploadTorrent(mi *metainfo.MetaInfo, concealUploaderIdentity bool) error {
	spec := torrent.TorrentSpecFromMetaInfo(mi)
	spec.Storage = me.uploadStorage
	t, new, err := me.torrentClient.AddTorrentSpec(spec)
	if err != nil {
		return err
	}
	if !new {
		panic("adding an upload should always be a new torrent")
	}
	// I think we're trying to avoid touching the network at all here, including announces etc. This
	// feature is currently supported in anacrolix/torrent? We could serve directly from the local
	// filesystem instead of going through the torrent storage if that was more appropriate?
	// TODO(anacrolix): Prevent network being touched at all, or bypass torrent client entirely.
	if concealUploaderIdentity {
		t.DisallowDataUpload()
	} else {
		me.addImplicitTrackers(t)
	}
	return nil
}

func (me *HttpHandler) handleUploads(rw InstrumentedResponseWriter, r *http.Request) error {
	resp := []objectInfo{} // Ensure not nil: I don't like 'null' as a response.
	err := service.IterUploads(me.uploadsDir, func(iu service.IteredUpload) {
		mi := iu.Metainfo
		err := iu.Err
		if err != nil {
			log.Errorf("error iterating uploads: %v", err)
			return
		}
		var oi objectInfo
		err = oi.FromUploadMetainfo(mi, iu.FileInfo.ModTime())
		if err != nil {
			log.Errorf("error parsing upload metainfo for %q: %v", iu.FileInfo.Name(), err)
			return
		}
		resp = append(resp, oi)
	})
	if err != nil {
		log.Errorf("error walking uploads dir: %v", err)
	}
	return encodeJsonResponse(rw, resp)
}

func (me *HttpHandler) handleDelete(rw InstrumentedResponseWriter, r *http.Request) (err error) {
	link := r.URL.Query().Get("link")
	m, err := metainfo.ParseMagnetUri(link)
	if err != nil {
		return handlerError{http.StatusBadRequest, errors.New("parsing magnet link: %v", err)}
	}

	var upload service.Upload
	if err := upload.FromMagnet(m); err != nil {
		log.Errorf("error getting upload spec from magnet link %q: %v", m, err)
		return handlerError{http.StatusBadRequest, errors.New("parsing replica uri: %v", err)}
	}

	// The prefixes returned from the uploads endpoint contain the file stem for the token file.
	uploadAuthFilePath := me.uploadTokenPath(upload.Prefix)
	authBytes, readAuthErr := ioutil.ReadFile(uploadAuthFilePath)

	metainfoFilePath := me.uploadMetainfoPath(upload)
	_, loadMetainfoErr := metainfo.LoadFromFile(metainfoFilePath)

	if readAuthErr == nil || loadMetainfoErr == nil {
		log.Debugf("deleting %q (auth=%q, haveMetainfo=%t)",
			upload.Prefix,
			string(authBytes),
			loadMetainfoErr == nil && os.IsNotExist(readAuthErr))
		// We're not inferring the endpoint from the link, should we?
		err := me.ReplicaServiceClient.DeleteUpload(
			upload.Prefix,
			string(authBytes),
			loadMetainfoErr == nil && os.IsNotExist(readAuthErr),
		)
		if err != nil {
			// It could be possible to unpack the service response status code and relay that.
			return errors.New("deleting upload: %v", err)
		}
		t, ok := me.torrentClient.Torrent(m.InfoHash)
		if ok {
			t.Drop()
		}
		os.RemoveAll(filepath.Join(me.dataDir, upload.String()))
		os.Remove(metainfoFilePath)
		os.Remove(uploadAuthFilePath)
	}
	if os.IsNotExist(loadMetainfoErr) && os.IsNotExist(readAuthErr) {
		return handlerError{http.StatusGone, errors.New("no upload tokens found")}
	}
	if !os.IsNotExist(readAuthErr) {
		return readAuthErr
	}
	return loadMetainfoErr
}

func copySpecificHeaders(dst, src http.Header, keys []string) {
	for _, k := range keys {
		for _, v := range src.Values(k) {
			dst.Add(k, v)
		}
	}
}

func (me *HttpHandler) handleMetadata(category string) func(InstrumentedResponseWriter, *http.Request) error {
	return func(rw InstrumentedResponseWriter, r *http.Request) error {
		query := r.URL.Query()
		replicaLink := query.Get("replicaLink")
		m, err := metainfo.ParseMagnetUri(replicaLink)
		if err != nil {
			return errors.New("parsing magnet link: %v", err)
		}
		so := m.Params.Get("so")
		fileIndex, err := strconv.ParseUint(so, 10, 0)
		if err != nil {
			err = errors.New("parsing so field: %v", err)
			log.Errorf("error %v", err)
			fileIndex = 0
		}
		mr := (&http.Request{
			Method: http.MethodGet,
			Header: make(http.Header),
		}).WithContext(r.Context())
		// These headers are only forwarded because we're going to forward the response straight
		// back to the browser!
		for _, h := range []string{
			"Accept",
			"Accept-Encoding",
			"Accept-Language",
			"Range",
		} {
			for _, v := range r.Header[h] {
				mr.Header.Add(h, v)
			}
		}
		gc := me.GlobalConfig()
		key := fmt.Sprintf("%s/%s/%d", m.InfoHash.HexString(), category, fileIndex)
		resp, err := doFirst(mr, me.HttpClient, func(r *http.Response) bool {
			return r.StatusCode/100 == 2
		}, func() (ret []string) {
			for _, s := range gc.GetMetadataBaseUrls() {
				ret = append(ret, s+key)
			}
			return
		}())
		if err != nil {
			if stdErrors.Is(err, r.Context().Err()) {
				return nil
			}
			return errors.New("doing http metadata request: %v", err)
		}
		defer resp.Body.Close()
		switch resp.StatusCode {
		case http.StatusOK, http.StatusPartialContent:
			copySpecificHeaders(rw.Header(), resp.Header, []string{
				"Content-Range", "Content-Length",
			})
			// Clobbering the origin's Cache-Control header.
			rw.Header().Set("Cache-Control", "public, max-age=604800, immutable")
			rw.WriteHeader(resp.StatusCode)
			_, err = io.Copy(rw, resp.Body)
			if err != nil {
				err = errors.New("copying metadata response: %v", err)
			}
			return err
		case http.StatusForbidden, http.StatusNotFound:
			// The behaviour I want here is to not retry for some small period of time. Chrome seems to ignore
			// this?
			rw.Header().Set("Cache-Control", "public, max-age=600")
		}
		rw.WriteHeader(resp.StatusCode)
		return resp.Body.Close()
	}
}

// Returns the first response for which the filter returns true. Otherwise a result is selected at
// random.
func doFirst(
	req *http.Request,
	client *http.Client,
	filter func(r *http.Response) bool,
	urls []string,
) (*http.Response, error) {
	if len(urls) == 0 {
		return nil, errors.New("no urls specified")
	}
	log.Debugf("trying urls %q", urls)
	type result struct {
		urlIndex int
		*http.Response
		error
	}
	results := make(chan result, len(urls))
	contextCancels := make([]func(), 0, len(urls))
	for i, url_ := range urls {
		ctx, cancel := context.WithCancel(req.Context())
		contextCancels = append(contextCancels, cancel)
		go func(i int, url_ string) {
			resp, err := func() (_ *http.Response, err error) {
				u, err := url.Parse(url_)
				if err != nil {
					return
				}
				req := req.Clone(ctx)
				req.RequestURI = ""
				req.URL = u
				req.Host = ""
				return client.Do(req)
			}()
			results <- result{i, resp, err}
		}(i, url_)
	}
	retChan := make(chan result)
	// Consumes all results, and ensures one is chosen to return.
	go func() {
		var rejected []result
		gotOne := false
		for range urls {
			res := <-results
			if !gotOne && res.error == nil && filter(res.Response) {
				retChan <- res
				gotOne = true
			} else {
				rejected = append(rejected, res)
			}
		}
		if !gotOne {
			// Return a randomly rejected result, and remove it from the slice.
			i := rand.Intn(len(rejected))
			retChan <- rejected[i]
			rejected[i] = rejected[len(rejected)-1]
			rejected = rejected[:len(rejected)-1]
		}
		// Cancel and close out all unused results.
		for _, res := range rejected {
			if res.error == nil {
				res.Body.Close()
			}
		}
	}()
	ret := <-retChan
	go func() {
		// Assert that no more returns are provided
		close(retChan)
		// Cancel everything we're not using
		for i, f := range contextCancels {
			if i != ret.urlIndex {
				f()
			}
		}
	}()
	log.Debugf("selected response from %q", urls[ret.urlIndex])
	return ret.Response, ret.error
}

func (me *HttpHandler) handleSearch(rw InstrumentedResponseWriter, r *http.Request) error {
	searchTerm := r.URL.Query().Get("s")

	rw.Set("search_term", searchTerm)
	// me.GaSession.EventWithLabel("replica", "search", searchTerm)
	if me.OnRequestReceived != nil {
		me.OnRequestReceived("search", searchTerm)
	}

	me.searchProxy.ServeHTTP(rw, r)
	return nil
}

func (me *HttpHandler) handleDownload(rw InstrumentedResponseWriter, r *http.Request) error {
	return me.handleViewWith(rw, r, "attachment")
}

func (me *HttpHandler) handleView(rw InstrumentedResponseWriter, r *http.Request) error {
	return me.handleViewWith(rw, r, "inline")
}

// This is extracted out so external packages can apply configs appropriately.
func ApplyReplicaOptions(ro ReplicaOptions, t *torrent.Torrent) {
	prefix := t.InfoHash().HexString()
	t.AddTrackers([][]string{ro.GetTrackers()})
	for _, peerAddr := range ro.GetStaticPeerAddrs() {
		t.AddPeers([]torrent.PeerInfo{{
			Addr:    torrent.StringAddr(peerAddr),
			Source:  torrent.PeerSourceDirect,
			Trusted: true,
		}})
	}
	t.UseSources(getMetainfoUrls(ro, prefix))
	t.AddWebSeeds(getWebseedUrls(ro, prefix))
}

func (me *HttpHandler) handleViewWith(rw InstrumentedResponseWriter, r *http.Request, inlineType string) error {
	rw.Set("inline_type", inlineType)

	link := r.URL.Query().Get("link")

	m, err := metainfo.ParseMagnetUri(link)
	if err != nil {
		return handlerError{http.StatusBadRequest, errors.New("parsing magnet link: %v", err)}
	}

	rw.Set("info_hash", m.InfoHash)

	t, _, release := me.confluence.GetTorrent(m.InfoHash)
	defer release()

	gc := me.GlobalConfig()

	log.Debugf("adding static peers: %q", gc.GetStaticPeerAddrs())

	if m.DisplayName != "" {
		t.SetDisplayName(m.DisplayName)
	}

	ApplyReplicaOptions(gc, t)

	selectOnly, err := strconv.ParseUint(m.Params.Get("so"), 10, 0)
	// Assume that it should be present, as it'll be added going forward where possible. When it's
	// missing, zero is a perfectly adequate default for now.
	if err != nil {
		log.Errorf("error parsing so field: %v", err)
	}
	// TODO <21-04-2022, soltzen> add a timeout to the context
	// https://github.com/getlantern/lantern-internal/issues/5483
	select {
	case <-r.Context().Done():
		// wrapHandlerError now adjusts log severity appropriately for context.Canceled.
		return r.Context().Err()
	case <-t.GotInfo():
	}
	filename := firstNonEmptyString(
		// Note that serving the torrent implies waiting for the info, and we could get a better
		// name for it after that. Torrent.Name will also allow us to reuse previously given 'dn'
		// values, if we don't have one now.
		m.DisplayName,
		t.Name(),
	)
	ext := path.Ext(filename)
	if ext != "" {
		filename = sanitize.BaseName(strings.TrimSuffix(filename, ext)) + ext
	}
	if filename != "" {
		rw.Header().Set("Content-Disposition", inlineType+"; filename*=UTF-8''"+url.QueryEscape(filename))
	}

	rw.Set("download_filename", filename)
	switch inlineType {
	case "inline":
		// me.GaSession.EventWithLabel("replica", "view", ext)
		if me.OnRequestReceived != nil {
			me.OnRequestReceived("view", ext)
		}
	case "attachment":
		// me.GaSession.EventWithLabel("replica", "download", ext)
		if me.OnRequestReceived != nil {
			me.OnRequestReceived("download", ext)
		}
	}

	torrentFile := t.Files()[selectOnly]
	fileReader := torrentFile.NewReader()
	defer fileReader.Close()
	rw.Header().Set("Cache-Control", "public, max-age=604800, immutable")
	confluence.ServeTorrentReader(rw, r, fileReader, torrentFile.Path())
	return nil
}

func metainfoUrls(link metainfo.Magnet, config ReplicaOptions) (ret []string) {
	ret = getMetainfoUrls(config, link.InfoHash.HexString())
	var upload service.Upload
	if upload.FromMagnet(link) == nil {
		ret = append(ret, getMetainfoUrls(config, upload.PrefixString())...)
	}
	return
}

func (me *HttpHandler) handleObjectInfo(rw InstrumentedResponseWriter, r *http.Request) error {
	query := r.URL.Query()
	replicaLink := query.Get("replicaLink")
	m, err := metainfo.ParseMagnetUri(replicaLink)
	if err != nil {
		return handlerError{http.StatusBadRequest, errors.New("parsing magnet link: %v", err)}
	}
	resp, err := doFirst(
		(&http.Request{}).WithContext(r.Context()),
		me.HttpClient,
		func(r *http.Response) bool {
			if r.StatusCode != http.StatusOK {
				return false
			}
			switch r.Header.Get("Content-Type") {
			case "application/x-bittorrent", "binary/octet-stream":
				return true
			default:
				return false
			}
		},
		metainfoUrls(m, me.GlobalConfig()),
	)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	mi, err := metainfo.Load(resp.Body)
	if err != nil {
		return err
	}

	metadata := make(map[string]interface{})
	metadata["creationDate"] = time.Unix(mi.CreationDate, 0).Format(time.RFC3339Nano)

	// Get metadata for torrent
	mr := (&http.Request{
		Method: http.MethodGet,
		Header: make(http.Header),
	}).WithContext(r.Context())
	mr.Header.Set("Accept", "application/json")

	gc := me.GlobalConfig()
	key := fmt.Sprintf("%s/metadata", m.InfoHash.HexString())

	resp, err = doFirst(
		mr, me.HttpClient,
		func(r *http.Response) bool {
			// Should we check for no encoding and JSON here?
			return r.StatusCode/100 == 2
		},
		func() (ret []string) {
			for _, s := range gc.GetMetadataBaseUrls() {
				ret = append(ret, s+key)
			}
			return
		}())

	// Whatever the response, we don't want the front-end to try again for a while.
	rw.Header().Set("Cache-Control", "public, max-age=600, immutable")
	if err == nil {
		defer resp.Body.Close()
		bodyBuf, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Errorf("error buffering metadata response for object info: %v", err)
			goto respond
		}
		err = json.NewDecoder(bytes.NewReader(bodyBuf)).Decode(&metadata)
		if err != nil {
			log.Errorf("decoding metadata json into object info: %v", err)
		}
	} else {
		if r.Context().Err() != nil {
			return err
		}
		log.Errorf("getting metadata for object info: %v", err)
	}
respond:
	return encodeJsonResponse(rw, metadata)
}

// What a bad language. TODO: cmp.Or this shit.
func firstNonEmptyString(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}

const uploadDirPerms = 0o750

// r is over the metainfo bytes.
func storeUploadedTorrent(r io.Reader, path string) error {
	err := os.MkdirAll(filepath.Dir(path), uploadDirPerms)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o640)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, r)
	if err != nil {
		return err
	}
	return f.Close()
}

func (me *HttpHandler) uploadMetainfoPath(upload service.Upload) string {
	return filepath.Join(me.uploadsDir, upload.String()+".torrent")
}

func (me *HttpHandler) uploadTokenPath(prefix service.Prefix) string {
	return filepath.Join(me.uploadsDir, prefix.PrefixString()+".token")
}

func encodeJsonResponse(rw http.ResponseWriter, resp interface{}) error {
	rw.Header().Set("Content-Type", "application/json")
	je := json.NewEncoder(rw)
	je.SetIndent("", "  ")
	je.SetEscapeHTML(false)
	if err := je.Encode(resp); err != nil {
		return encoderWriterError{err}
	}
	return nil
}

func encodeJsonErrorResponse(rw http.ResponseWriter, resp interface{}, statusCode int) error {
	// necessary here because of header writing ordering requirements
	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(statusCode)
	return encodeJsonResponse(rw, resp)
}

func (me *HttpHandler) addImplicitTrackers(t *torrent.Torrent) {
	t.AddTrackers([][]string{me.GlobalConfig().GetTrackers()})
}

func (me *HttpHandler) metricsExporter() {
	for {
		me.doMetricsOp()
		select {
		case <-me.closed.Done():
			return
		case <-time.After(5 * time.Minute):
		}
	}
}

func (me *HttpHandler) doMetricsOp() {
	op := ops.Begin("replica_metrics")
	stats := me.torrentClient.Stats()
	walkFieldsForMetrics(
		func(path []string, value any) {
			op.Set(
				strings.Join(append([]string{"HttpHandler", "torrentClient", "Stats"}, path...), "."),
				value,
			)
		},
		reflect.ValueOf(stats),
		nil,
	)
	op.End()
	log.Debugf("exported replica metrics")
}

func walkFieldsForMetrics(f func(path []string, value any), rv reflect.Value, path []string) {
	i := rv.Interface()
	if count, ok := i.(torrent.Count); ok {
		f(path, count.Int64())
		return
	}
	switch rv.Kind() {
	case reflect.Struct:
		//log.Debugf("walking struct in %v", path)
		for fieldIndex := 0; fieldIndex < rv.NumField(); fieldIndex++ {
			sf := rv.Type().Field(fieldIndex)
			if !sf.IsExported() {
				continue
			}
			walkFieldsForMetrics(f, rv.FieldByIndex(sf.Index), append(path, sf.Name))
		}
	default:
		f(path, rv.Interface())
	}
}
