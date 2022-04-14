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
	"strconv"
	"strings"
	"sync"
	"time"

	"crawshaw.io/sqlite"
	"crawshaw.io/sqlite/sqlitex"
	"github.com/anacrolix/missinggo"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/getlantern/dhtup"
	"github.com/getlantern/eventual/v2"
	"github.com/getlantern/replica"
	"github.com/getlantern/replica/service"
)

type LocalIndexDhtDownloader struct {
	// FullyDownloadedLocalIndexPath to the latest fully-downloaded local index
	FullyDownloadedLocalIndexPath eventual.Value

	res                    dhtup.Resource
	lastSuccessfulInfohash metainfo.Hash
}

func RunLocalIndexDownloader(
	res dhtup.Resource,
	checkForNewUpdatesEvery time.Duration,
) *LocalIndexDhtDownloader {
	l := &LocalIndexDhtDownloader{
		res:                           res,
		FullyDownloadedLocalIndexPath: eventual.NewValue(),
	}
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
				"ignorable error while downloading local index. Will try again in %v: %v",
				checkForNewUpdatesEvery, err)
			// If an error occurs, retry immediately
			cancel()
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
	// Check if there's a new infohash to download (or download the last
	// infohash if this is our first run)
	bep46PayloadInfohash, err := me.res.FetchBep46Payload(ctx)
	if err != nil {
		return log.Errorf("while fetching bep46 payload: %v", err), false
	}
	if bytes.Equal(me.lastSuccessfulInfohash[:], bep46PayloadInfohash[:]) {
		// No new infohash
		return nil, false
	}

	// Fetch the torrent's io.ReadCloser and attempt to download it with
	// io.Copy
	r, _, err := me.res.FetchTorrentFileReader(ctx, bep46PayloadInfohash)
	if err != nil {
		return err, false
	}
	ctxReader := missinggo.ContextedReader{R: r, Ctx: ctx}
	fd, err := os.CreateTemp("", "replica-local-index")
	if err != nil {
		return log.Errorf("Making local index file: %v", err), false
	}
	defer fd.Close()
	// TODO <23-03-22, soltzen> Should we have a context-aware reader here?
	_, err = io.Copy(fd, ctxReader)
	if err != nil {
		return log.Errorf("Copying local index torrent reader: %v", err), false
	}

	// Before replacing with the new value, see if there's already a value in
	// FullyDownloadedLocalIndexPath. If so, reset the eventual.Value to avoid race conditions and
	// unlink the file
	expiredCtx, cancel := context.WithCancel(context.Background())
	cancel()
	v, _ := me.FullyDownloadedLocalIndexPath.Get(expiredCtx)
	// XXX <28-03-2022, soltzen> We don't call what's the error: just care
	// whether there's a value set
	if v != nil {
		me.FullyDownloadedLocalIndexPath.Reset()
		os.Remove(v.(string))
	}

	me.FullyDownloadedLocalIndexPath.Set(fd.Name())
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
	dbpool, err := sqlitex.Open(fmt.Sprintf("file:%s?mode=rw", localIndexPath), 0, 1)
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
