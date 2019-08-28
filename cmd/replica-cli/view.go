package main

import (
	"bytes"
	"html/template"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/skratchdot/open-golang/open"
	"golang.org/x/xerrors"
)

func viewTorrent(torrentFile string, debugClient bool) error {
	cfg := torrent.NewDefaultClientConfig()
	cfg.Debug = debugClient
	cfg.HeaderObfuscationPolicy.Preferred = false
	cl, err := torrent.NewClient(cfg)
	if err != nil {
		return xerrors.Errorf("creating torrent client: %w", err)
	}
	defer cl.Close()
	http.HandleFunc("/torrentClientStatus", func(w http.ResponseWriter, r *http.Request) {
		cl.WriteStatus(w)
	})
	tor, err := cl.AddTorrentFromFile(torrentFile)
	if err != nil {
		return xerrors.Errorf("adding torrent to client: %w", err)
	}

	// This can be expensive for end-users, but is helpful when testing when file names are used but
	// the data changes.

	//tor.VerifyData()

	mux := http.NewServeMux()
	setupMux(mux, tor)
	l, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return xerrors.Errorf("creating http server listener: %w", err)
	}
	defer l.Close()
	log.Printf("http server at %v", l.Addr())
	serveErr := make(chan error)
	go func() { serveErr <- http.Serve(l, mux) }()
	if err := open.Run("http://" + l.Addr().String()); err != nil {
		return xerrors.Errorf("opening content: %w", err)
	}
	select {
	case err := <-serveErr:
		if err != nil {
			return xerrors.Errorf("http server: %w", err)
		}
		return nil
	}
}

func setupMux(m *http.ServeMux, t *torrent.Torrent) {
	for _, f := range t.Files() {
		m.Handle("/"+f.DisplayPath(), fileHandler(f))
	}
	m.Handle("/", contentsHandler(t))
}

var contentPageTemplate = template.Must(template.New("contents").Parse(`
{{ range . }}
	<a href="/{{ .DisplayPath }}">{{ .DisplayPath }}</a><br>
{{ end }}
`))

func contentsHandler(t *torrent.Torrent) http.HandlerFunc {
	var buf bytes.Buffer
	if err := contentPageTemplate.Execute(&buf, t.Files()); err != nil {
		panic(err)
	}
	return func(w http.ResponseWriter, r *http.Request) {
		w.Write(buf.Bytes())
	}
}

func fileHandler(f *torrent.File) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		r := f.NewReader()
		defer r.Close()
		http.ServeContent(w, req, "", time.Time{}, r)
	}
}
