package main

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/anacrolix/log"
	"github.com/anacrolix/squirrel"
	"github.com/anacrolix/torrent"
	sqliteStorage "github.com/anacrolix/torrent/storage/sqlite"
	"github.com/anacrolix/torrent/types/infohash"
	"github.com/davecgh/go-spew/spew"
	"github.com/getlantern/flashlight/v7/embeddedconfig"
	"github.com/redis/go-redis/v9"

	"github.com/getlantern/replica/server"
)

func main() {
	spew.Dump(embeddedconfig.GlobalReplicaOptions)
	rc := redis.NewClient(&redis.Options{
		Addr:        "replica-redis.internal:6379",
		ReadTimeout: 10 * time.Second,
	})
	defer rc.Close()
	ctx := context.Background()
	cfg := torrent.NewDefaultClientConfig()
	cacheOpts := squirrel.NewCacheOpts{}
	cacheOpts.Path = "squirrel.db"
	squirrelCache, err := squirrel.NewCache(cacheOpts)
	if err != nil {
		panic(err)
	}
	defer squirrelCache.Close()
	cfg.DefaultStorage = sqliteStorage.NewWrappingClient(squirrelCache)
	tc, err := torrent.NewClient(cfg)
	if err != nil {
		panic(err)
	}
	defer tc.Close()
	http.HandleFunc("/torrentClientStatus", func(w http.ResponseWriter, r *http.Request) {
		tc.WriteStatus(w)
	})
	pageTmpl := template.New("")
	_, err = pageTmpl.Parse(`
		{{ range . }}
			{{ $torrent := . }}
			<p>{{ if .Info }}{{ .Info.Name }}{{ else }}{{ .InfoHash }}{{ end }}: {{ .EventAge }}</p>
			{{ if .Info }}
				<ul>
					{{ range $fileIndex, $file := .Files }}
						<li><a href="/{{ $torrent.InfoHash }}/{{ $fileIndex }}">{{ .DisplayPath }}</a></li>
					{{ end }}
				</ul>
			{{ end }}
		{{ end }}
	`)
	if err != nil {
		panic(err)
	}
	handleFile := func(w http.ResponseWriter, r *http.Request) bool {
		pathParts := strings.Split(strings.TrimLeft(r.URL.Path, "/"), "/")
		if len(pathParts) != 2 {
			return false
		}
		var ih infohash.T
		err := ih.FromHexString(pathParts[0])
		if err != nil {
			panic(err)
		}
		fileIndex, err := strconv.ParseInt(pathParts[1], 0, 0)
		if err != nil {
			panic(err)
		}
		t, _ := tc.Torrent(ih)
		torrentFile := t.Files()[fileIndex]
		fileReader := torrentFile.NewReader()
		defer fileReader.Close()
		http.ServeContent(w, r, torrentFile.Path(), time.Time{}, fileReader)
		return true
	}
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("serving %q", r.URL)
		if handleFile(w, r) {
			return
		}
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		events := rc.XRevRangeN(ctx, "uploads:cloudflare-replica", "+", "-", 10000)
		if events.Err() != nil {
			panic(events.Err())
		}
		var wg sync.WaitGroup
		const maxTorrents = 30
		userTorrents := make(chan pageTorrent, maxTorrents)
		getInfoSem := make(chan struct{}, 10)
		for _, event := range events.Val() {
			hexStr := event.Values["prefix"].(string)
			epochMillisStr := strings.Split(event.ID, "-")[0]
			epochMillis, err := strconv.ParseInt(epochMillisStr, 0, 64)
			if err != nil {
				panic(err)
			}
			eventTime := time.UnixMilli(epochMillis)
			var ih infohash.T
			err = ih.FromHexString(hexStr)
			if err != nil {
				panic(err)
			}
			addOpts := torrent.AddTorrentOpts{
				InfoHash: ih,
			}
			infoBytesKey := fmt.Sprintf("infobytes/%s", ih.HexString())
			infoWasCached := false
			// There is a Cache.ReadAll method now... Keeping this for now in case versions need to
			// be rewound.
			pb, err := squirrelCache.OpenPinnedReadOnly(infoBytesKey)
			if err == nil {
				infoBytes, err := io.ReadAll(io.NewSectionReader(pb, 0, pb.Length()))
				if err != nil {
					panic(err)
				}
				addOpts.InfoBytes = infoBytes
				infoWasCached = true
			} else if errors.Is(err, fs.ErrNotExist) {
				getInfoSem <- struct{}{}
			} else {
				panic(err)
			}
			t, new := tc.AddTorrentOpt(addOpts)
			if new {
				server.ApplyReplicaOptions(&embeddedconfig.GlobalReplicaOptions, t)
			}
			wg.Add(1)
			started := time.Now()
			ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
			go func() {
				select {
				case <-t.GotInfo():
					if new && !infoWasCached {
						log.Printf("got info for %v after %v", t.InfoHash().HexString(), time.Since(started))
						err = squirrelCache.Put(infoBytesKey, t.Metainfo().InfoBytes)
						if err != nil {
							panic(err)
						}
					}
					if !strings.HasPrefix(t.Info().Name, "youtube") {
						log.Printf("got user upload from %v ago", time.Since(eventTime))
						select {
						case userTorrents <- pageTorrent{
							Torrent:  t,
							EventAge: time.Second * time.Duration(math.Floor(time.Since(eventTime).Seconds())),
						}:
						default:
						}
					}
				case <-ctx.Done():
					log.Printf("getting info for %v: %v", t.InfoHash().HexString(), ctx.Err())
				}
				cancel()
				wg.Done()
				if !infoWasCached {
					<-getInfoSem
				}
			}()
			if r.Context().Err() != nil {
				return
			}
			if len(userTorrents) == maxTorrents {
				break
			}
		}
		wg.Wait()
		close(userTorrents)
		userTorrentsSlice := make([]pageTorrent, 0, len(userTorrents))
		for t := range userTorrents {
			userTorrentsSlice = append(userTorrentsSlice, t)
		}
		err := pageTmpl.Execute(w, userTorrentsSlice)
		if err != nil {
			panic(err)
		}
	})
	panic(http.ListenAndServe(":80", nil))
}

type pageTorrent struct {
	*torrent.Torrent
	EventAge time.Duration
}
