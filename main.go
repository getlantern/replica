package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	_ "github.com/anacrolix/envpprof"
	"github.com/anacrolix/torrent"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	cli "github.com/jawher/mow.cli"
	"github.com/skratchdot/open-golang/open"
	"golang.org/x/xerrors"
)

const bucket = "getlantern-replica"

func main() {
	err := mainErr()
	if err != nil {
		log.Fatalf("error in main: %v", err)
	}
}

func checkAction(err error) {
	if err == nil {
		return
	}
	log.Printf("error: %v", err)
	cli.Exit(1)
}

func mainErr() error {
	app := cli.App("replica", "Lantern Replica functions")
	app.Command("upload", "uploads a file to S3 and returns the S3 key", func(cmd *cli.Cmd) {
		file := cmd.StringArg("FILE", "", "file to upload")
		cmd.Action = func() {
			checkAction(uploadFile(*file))
		}
	})
	app.Command("get-torrent", "retrieve BitTorrent metainfo for a Replica S3 key", func(cmd *cli.Cmd) {
		name := cmd.StringArg("NAME", "", "Replica S3 object name")
		cmd.Action = func() { checkAction(getTorrent(*name)) }
	})
	app.Command("open-torrent", "open torrent contents", func(cmd *cli.Cmd) {
		file := cmd.StringArg("FILE", "", "torrent to open")
		index := cmd.IntOpt("index i", 0, "torrent file index to open for multi-file torrents")
		debug := cmd.BoolOpt("debug d", false, "debug torrent client")
		cmd.Action = func() { checkAction(viewTorrent(*file, *index, *debug)) }
	})
	return app.Run(os.Args)
}

func newSession() *session.Session {
	return session.Must(session.NewSession(&aws.Config{
		Region: aws.String("ap-southeast-1"),
	}))
}

func upload(args []string) error {
	return uploadFile(args[0])
}

func uploadFile(filename string) error {
	sess := newSession()

	// Create an uploader with the session and default options
	uploader := s3manager.NewUploader(sess)

	f, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("failed to open file %q: %v", filename, err)
	}

	s3Key := filepath.Base(filename)

	// Upload the file to S3.
	result, err := uploader.Upload(&s3manager.UploadInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(s3Key),
		Body:   f,
	})
	if err != nil {
		return xerrors.Errorf("failed to upload file, %w", err)
	}
	log.Printf("file uploaded to %q\n", result.Location)
	fmt.Println(s3Key)
	return nil
}

func getTorrent(filename string) error {
	sess := newSession()
	svc := s3.New(sess)
	out, err := svc.GetObjectTorrent(&s3.GetObjectTorrentInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(filename),
	})
	if err != nil {
		return err
	}
	defer out.Body.Close()
	f, err := os.OpenFile(filepath.Base(filename)+".torrent", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0640)
	if err != nil {
		return xerrors.Errorf("opening output file: %w", err)
	}
	log.Printf("created %q", f.Name())
	defer f.Close()
	if _, err := io.Copy(f, out.Body); err != nil {
		return xerrors.Errorf("copying torrent: %w", err)
	}
	if err := f.Close(); err != nil {
		return xerrors.Errorf("closing torrent file: %w", f.Close())
	}
	return nil
}

func viewTorrent(torrentFile string, fileIndex int, debugClient bool) error {
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

	if fileIndex < 0 || fileIndex >= len(tor.Files()) {
		return xerrors.Errorf("file index out of bounds (torrent has %d files)", len(tor.Files()))
	}

	// This can be expensive for end-users, but is helpful when testing when file names are used but
	// the data changes.

	//tor.VerifyData()

	http.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		r := tor.Files()[fileIndex].NewReader()
		defer r.Close()
		http.ServeContent(w, req, tor.Info().Name, time.Time{}, r)
	})
	l, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return xerrors.Errorf("creating http server listener: %w", err)
	}
	defer l.Close()
	serveErr := make(chan error)
	go func() { serveErr <- http.Serve(l, nil) }()
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
