package main

import (
	"context"
	"github.com/anacrolix/log"
	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/types/infohash"
	"github.com/davecgh/go-spew/spew"
	"github.com/getlantern/flashlight/embeddedconfig"
	"github.com/getlantern/replica/server"
	"github.com/redis/go-redis/v9"
	"html/template"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

func main() {
	spew.Dump(embeddedconfig.GlobalReplicaOptions)
	rc := redis.NewClient(&redis.Options{
		Addr: "replica-redis.internal:6379",
	})
	defer rc.Close()
	ctx := context.Background()
	tc, err := torrent.NewClient(nil)
	if err != nil {
		panic(err)
	}
	defer tc.Close()
	pageTmpl := template.New("")
	_, err = pageTmpl.Parse(`
		{{ range .Torrents }}
			{{ $torrent := . }}
			<p>{{ if .Info }}{{ .Info.Name }}{{ else }}{{ .InfoHash }}{{ end }}</p>
			{{ if .Info }}
				<ul>
					{{ range $fileIndex, $file := .Files }}
						<li><a href="/{{ $torrent.InfoHash }}/{{ $fileIndex }}">{{ .Path }}</a></li>
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
		if handleFile(w, r) {
			return
		}
		events := rc.XRevRangeN(ctx, "uploads:cloudflare-replica", "+", "-", 20)
		if events.Err() != nil {
			panic(events.Err())
		}
		var wg sync.WaitGroup
		for _, event := range events.Val() {
			hexStr := event.Values["prefix"].(string)
			var ih infohash.T
			err := ih.FromHexString(hexStr)
			if err != nil {
				panic(err)
			}
			t, new := tc.AddTorrentInfoHash(ih)
			if new {
				server.ApplyReplicaOptions(&embeddedconfig.GlobalReplicaOptions, t)
			}
			wg.Add(1)
			ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
			go func() {
				select {
				case <-t.GotInfo():
					if new {
						log.Printf("got info for %v", t.InfoHash().HexString())
					}
				case <-ctx.Done():
					log.Printf("getting info for %v: %v", t.InfoHash().HexString(), ctx.Err())
				}
				cancel()
				wg.Done()
			}()
		}
		wg.Wait()
		err := pageTmpl.Execute(w, tc)
		if err != nil {
			panic(err)
		}
	})
	panic(http.ListenAndServe(":80", nil))
}
