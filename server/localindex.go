package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"crawshaw.io/sqlite"
	"crawshaw.io/sqlite/sqlitex"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/getlantern/replica"
	"github.com/getlantern/replica/service"
)

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

type LocalIndexIndexRoundTripper struct {
	input NewHttpHandlerInput
	ctx   context.Context
}

func (a *LocalIndexIndexRoundTripper) makeLocalIndexIndexResponse(
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

func (a *LocalIndexIndexRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	// If we don't have a backup index, just return
	if a.input.LocalIndexDhtupResource == nil {
		return nil, nil
	}

	r, _, err := a.input.LocalIndexDhtupResource.Open(a.ctx)
	if err != nil {
		return nil, log.Errorf("opening dht resource: %v", err)
	}
	defer r.Close()

	// TODO <15-03-22, soltzen> Do some magic here to turn this
	// (io.ReadCloser)(r) into a file path to a fully-downloaded sqlite db
	p := ""

	// p, ok := a.input.LocalIndexPath.Get(a.input.LocalIndexPathFetchTimeout)
	// if !ok {
	// 	return nil, nil
	// }

	select {
	case <-req.Context().Done():
		return nil, req.Context().Err()
	default:
	}

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
		req.Context(), p, q, limit, offset)
	if err != nil {
		return nil, log.Errorf(
			"while fetching search results from local index: %v", err)
	}
	respBody, err := a.makeLocalIndexIndexResponse(req, results, limit, offset)
	if err != nil {
		return nil, log.Errorf(" %v", err)
	}

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
