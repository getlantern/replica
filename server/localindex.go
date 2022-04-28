package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"crawshaw.io/sqlite"
	"crawshaw.io/sqlite/sqlitex"
	"github.com/anacrolix/missinggo/v2"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/getlantern/dhtup"
	"github.com/getlantern/eventual/v2"
	"github.com/getlantern/replica"
	"github.com/getlantern/replica/service"
)

const LocalIndexFilenamePrefix = "replica-local-index"

// Taken from https://gist.github.com/var23rav/23ae5d0d4d830aff886c3c970b8f6c6b
// Because os.Rename() across directories will produce an "invalid cross-device
// link" error
func moveFile(sourcePath, destPath string) error {
	inputFile, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("Couldn't open source file: %s", err)
	}
	outputFile, err := os.Create(destPath)
	if err != nil {
		inputFile.Close()
		return fmt.Errorf("Couldn't open dest file: %s", err)
	}
	defer outputFile.Close()
	_, err = io.Copy(outputFile, inputFile)
	inputFile.Close()
	if err != nil {
		return fmt.Errorf("Writing to output file failed: %s", err)
	}
	// The copy was successful, so now delete the original file
	err = os.Remove(sourcePath)
	if err != nil {
		return fmt.Errorf("Failed removing original file: %s", err)
	}
	return nil
}

type LocalIndexDhtDownloader struct {
	// FullyDownloadedLocalIndexPath to the latest fully-downloaded local index
	FullyDownloadedLocalIndexPath eventual.Value

	configDir              string
	res                    dhtup.Resource
	lastSuccessfulInfohash metainfo.Hash
}

// RunLocalIndexDownloader does three things:
// - Scans configDir for local index sqlite DBs that were already downloaded
//   and assign it to FullyDownloadedLocalIndexPath
// - Downloads the latest local index from the DHT
func RunLocalIndexDownloader(
	res dhtup.Resource,
	checkForNewUpdatesEvery time.Duration,
	configDir string,
) *LocalIndexDhtDownloader {
	l := &LocalIndexDhtDownloader{
		res:                           res,
		FullyDownloadedLocalIndexPath: eventual.NewValue(),
		configDir:                     configDir,
	}

	// Walk through the config directory and use the last fully-downloaded
	// local index as the initial value
	r, err := regexp.Compile("replica-local-index.*sqlite$")
	if err != nil {
		// This is a programmer error: better panic than silently fail in the
		// logs
		panic(fmt.Sprintf("Couldn't compile regexp: %v", err))
	}
	filepath.Walk(l.configDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if r.MatchString(info.Name()) {
			// log.Debugf("Found local index file %v", path)
			l.FullyDownloadedLocalIndexPath.Set(path)
		}
		return nil
	})

	l.runDownloadRoutine(checkForNewUpdatesEvery)
	return l
}

// runDownloadRoutine calls download() in a loop every "checkForNewUpdatesEvery".
func (me *LocalIndexDhtDownloader) runDownloadRoutine(checkForNewUpdatesEvery time.Duration) {
	go func() {
	looper:
		ctx, cancel := context.WithTimeout(context.Background(), checkForNewUpdatesEvery)
		err, hasNewIndex := me.download(ctx)
		if err != nil {
			log.Debugf(
				"ignorable error while downloading local index. Will try again immediately: %v",
				err)
			// If an error occurs, wait a bit before trying again.
			cancel()
			time.Sleep(checkForNewUpdatesEvery / 2)
			goto looper
		}
		// Else, re-run every "checkForNewUpdatesEvery"
		if !hasNewIndex {
			log.Debugf("No new infohash since the last one [%s] for local index sqlite DB. Sleeping for a bit and checking for updates in %v",
				me.lastSuccessfulInfohash.HexString(), checkForNewUpdatesEvery)
		} else {
			log.Debugf("Successfully downloaded new local index [%s]. Sleeping for a bit and checking for updates in %v",
				me.lastSuccessfulInfohash.HexString(), checkForNewUpdatesEvery)
		}
		cancel()
		time.Sleep(checkForNewUpdatesEvery)
		goto looper
	}()
}

// download fetches the bep46 payload for this resource and checks if it's
// already been downloaded. If not, attempt to download this resource somewhere
// in a temporary directory and set (or replace) the
// LocalIndexDhtDownloader.FullyDownloadedLocalIndexPath value with the latest value.
func (me *LocalIndexDhtDownloader) download(ctx context.Context) (err error, hasNewIndex bool) {
	log.Debugf("Attempting to fetch Bep46 of Replica local index")
	// Check if there's a new infohash to download (or download the last
	// infohash if this is our first run)
	bep46PayloadInfohash, err := me.res.FetchBep46Payload(ctx)
	if err != nil {
		return log.Errorf("while fetching bep46 payload: %v", err), false
	}
	// TODO <14-04-2022, soltzen> There's a weird edge case here:
	// - if there was a downloaded local index file,
	// - and it matches the infohash of the bep46 payload,
	// - we'll still retrigger the download.
	//
	// This happens since we don't save the last infohash we downloaded. We
	// really should. The optimization here is that this'll save the user a
	// single download upon launch
	if bytes.Equal(me.lastSuccessfulInfohash[:], bep46PayloadInfohash[:]) {
		// No new infohash
		return nil, false
	}
	log.Debugf("Fetched new infohash for Replica local index %s",
		bep46PayloadInfohash.HexString())

	// Fetch the torrent's reader and attempt to download it with
	// io.Copy
	r, _, err := me.res.FetchTorrentFileReader(ctx, bep46PayloadInfohash)
	if err != nil {
		return err, false
	}
	defer r.Close()
	log.Debugf(
		"Fetched torrent io.reader for Replica local index %s. Attempting to download...",
		bep46PayloadInfohash.HexString())
	ctxReader := missinggo.ContextedReader{R: r, Ctx: ctx}
	fd, err := os.CreateTemp("", "replica-local-index")
	if err != nil {
		return log.Errorf("making local index file: %v", err), false
	}
	defer fd.Close()
	_, err = io.Copy(fd, ctxReader)
	if err != nil {
		os.Remove(fd.Name())
		return log.Errorf("downloading local index torrent reader: %v", err), false
	}

	// Before replacing with the new value, see if there's already a value in
	// FullyDownloadedLocalIndexPath. If so, reset the eventual.Value to avoid
	// race conditions and unlink the file
	expiredCtx, cancel := context.WithCancel(context.Background())
	cancel()
	v, _ := me.FullyDownloadedLocalIndexPath.Get(expiredCtx)
	// XXX <28-03-2022, soltzen> We don't call what's the error: just care
	// whether there's a value set
	if v != nil {
		log.Debugf("Found old Replica local index file. Unlinking...")
		me.FullyDownloadedLocalIndexPath.Reset()
		os.Remove(v.(string))
	}

	// Move the file to configDir. This is necessary so that the initial
	// startup of Lantern picks it up instead of waiting for a redownload.
	// Also, name it with LocalIndexFilenamePrefix so that it's picked up by
	// filepath.Walk() and assigned to me.FullyDownloadedLocalIndexPath as the
	// initial value, if it exists
	n := time.Now()
	newPath := filepath.Join(me.configDir,
		fmt.Sprintf("/%s-%s.sqlite",
			LocalIndexFilenamePrefix,
			n.Format("20060102-150405")))
	err = moveFile(fd.Name(), newPath)
	if err != nil {
		os.Remove(fd.Name())
		return log.Errorf("moving downloaded local index file from %s to %s: %v",
			fd.Name(), newPath, err), false
	}

	log.Debugf(
		"Successfully downloaded Replica local index %s. Setting path to %v",
		bep46PayloadInfohash.HexString(), newPath)
	me.FullyDownloadedLocalIndexPath.Set(newPath)
	return nil, true
}

func encloseFts5QueryString(s string) string {
	return `"` + strings.Replace(s, `"`, `""`, -1) + `"`
}

type SearchResult struct {
	HostedOnReplica         bool     `json:"hostedOnReplica"`
	InfoHash                string   `json:"infoHash"`
	TorrentInternalFilePath string   `json:"torrentInternalFilePath"`
	FileSize                int64    `json:"fileSize"`
	TorrentName             string   `json:"torrentName"`
	MimeTypes               []string `json:"mimeTypes"`
	LastModified            string   `json:"lastModified"`
	ReplicaLink             string   `json:"replicaLink"`
	TorrentNumFiles         uint     `json:"torrentNumFiles"`
	DisplayName             string   `json:"displayName"`
	// always null
	SwarmMetadata    *string `json:"swarmMetadata"`
	UploadSearchRank float64 `json:"uploadSearchRank"`
	SourceIndex      string  `json:"sourceIndex"`
}

type RespBody struct {
	Results      []*SearchResult `json:"objects"`
	TotalResults int             `json:"totalResults"`
	StartIndex   int             `json:"startIndex"`
	ItemsPerPage int             `json:"itemsPerPage"`
}

func NewSearchResult(stmt *sqlite.Stmt) (*SearchResult, error) {
	prefix := stmt.GetText("prefix")
	rank := stmt.GetFloat("rank")
	path := stmt.GetText("path")
	length := stmt.GetInt64("length")
	creationDate := stmt.GetInt64("creation_date")
	infohash := stmt.GetText("info_hash")
	infoname := stmt.GetText("info_name")
	var ih metainfo.Hash
	if err := ih.FromHexString(infohash); err != nil {
		return nil, log.Errorf("failed to make replicaLink %v", err)
	}

	return &SearchResult{
		HostedOnReplica:         true,
		InfoHash:                infohash,
		TorrentInternalFilePath: path,
		FileSize:                length,
		TorrentName:             infoname,
		MimeTypes:               []string{},
		LastModified: time.
			Unix(creationDate, 0).
			Format(time.RFC3339),
		ReplicaLink:      replica.CreateLink(ih, service.Prefix(prefix), []string{path}),
		TorrentNumFiles:  1,
		DisplayName:      path,
		SwarmMetadata:    nil,
		UploadSearchRank: rank,
		SourceIndex:      "replica",
	}, nil
}

type LocalIndexRoundTripper struct {
	localIndexDhtDownloader *LocalIndexDhtDownloader
}

func (a *LocalIndexRoundTripper) makeLocalIndexResponse(
	req *http.Request,
	results []*SearchResult,
	limit int,
	offset int,
) ([]byte, error) {
	ret := RespBody{
		Results:      results,
		TotalResults: len(results),
		StartIndex:   offset,
		ItemsPerPage: limit,
	}
	b, err := json.Marshal(ret)
	if err != nil {
		return nil, log.Errorf(" %v", err)
	}
	return b, nil
}

func (a *LocalIndexRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	// If we don't have a backup index, just return.
	// This should never happen, but just in case
	if a.localIndexDhtDownloader == nil {
		return nil, log.Errorf("No local index dhtup resource specified")
	}

	// This function guarantees that the fetched local index will be
	// fully-downloaded to the file system
	fp, err := a.localIndexDhtDownloader.FullyDownloadedLocalIndexPath.Get(req.Context())
	if err != nil {
		return nil, log.Errorf("while opening dht resource: %v", err)
	}

	// Prep parameters, call the search index, and prepare response object
	q := req.URL.Query().Get("s")
	offset, err := strconv.Atoi(req.URL.Query().Get("offset"))
	if err != nil {
		offset = 0
	}
	limit, err := strconv.Atoi(req.URL.Query().Get("limit"))
	if err != nil {
		limit = 20
	}
	results, err := fetchSearchResultsFromLocalIndex(
		req.Context(), fp.(string), q, limit, offset)
	if err != nil {
		return nil, log.Errorf(
			"while fetching search results from local index: %v", err)
	}
	respBody, err := a.makeLocalIndexResponse(req, results, limit, offset)
	if err != nil {
		return nil, log.Errorf(" %v", err)
	}

	// One last check to see if our context died
	select {
	case <-req.Context().Done():
		return nil, req.Context().Err()
	default:
	}

	return &http.Response{
		Status:        "200 OK",
		StatusCode:    200,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Body:          io.NopCloser(bytes.NewReader(respBody)),
		ContentLength: int64(len(respBody)),
		Request:       req,
		Header:        make(http.Header, 0),
	}, nil
}

func fetchSearchResultsFromLocalIndex(
	ctx context.Context,
	localIndexPath, searchQuery string,
	limit, offset int) ([]*SearchResult, error) {
	p := fmt.Sprintf("file:%s?mode=rw", localIndexPath)
	log.Debugf("Opening local index %s", p)
	dbpool, err := sqlitex.Open(p, 0, 1)
	if err != nil {
		return nil, log.Errorf(
			"while opening connection to local index: %v", err)
	}
	conn := dbpool.Get(ctx)
	if conn == nil {
		return nil, log.Errorf("while getting sqlite conn: no connections left")
	}
	defer dbpool.Put(conn)

	stmt := conn.Prep(
		"SELECT prefix, creation_date, info_hash, info_name, path, length, rank FROM upload_fts(?) LIMIT ? OFFSET ?",
	)
	stmt.BindText(1, encloseFts5QueryString(searchQuery))
	stmt.BindInt64(2, int64(limit))
	stmt.BindInt64(3, int64(offset))
	results := []*SearchResult{}
	for {
		if hasRow, err := stmt.Step(); err != nil {
			log.Debugf(
				"Ignorable failure to step while doing FTS on backup replica index: %v",
				err)
			continue
		} else if !hasRow {
			break
		}
		sr, err := NewSearchResult(stmt)
		if err != nil {
			log.Debugf(
				"Ignorable failure to make new search result in backup replica index: %v",
				err)
			continue
		}
		results = append(results, sr)
	}
	return results, nil
}
