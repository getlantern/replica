package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/skratchdot/open-golang/open"
	"github.com/spf13/cobra"
	"golang.org/x/xerrors"
)

const bucket = "getlantern-replica"

func main() {
	err := mainErr()
	if err != nil {
		log.Fatal(err)
	}
}

func mainErr() error {
	cmd := cobra.Command{}
	cmd.AddCommand(
		&cobra.Command{
			Use:  "upload FILE",
			Args: cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				return uploadFile(args[0])
			},
		},
		&cobra.Command{
			Use: "get-torrent FILE",
			RunE: func(cmd *cobra.Command, args []string) error {
				return getTorrent(args[0])
			},
			Args: cobra.ExactArgs(1),
		},
		&cobra.Command{
			Use: "view-torrent FILE",
			RunE: func(cmd *cobra.Command, args []string) error {
				log.Print(viewTorrent(args[0]))
				return nil
			},
			Args: cobra.ExactArgs(1),
		},
	)
	return cmd.Execute()
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
		return fmt.Errorf("failed to open file %q, %v", filename, err)
	}

	// Upload the file to S3.
	result, err := uploader.Upload(&s3manager.UploadInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(filename),
		Body:   f,
	})
	if err != nil {
		return fmt.Errorf("failed to upload file, %v", err)
	}
	log.Printf("file uploaded to, %s\n", result.Location)
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
	f, err := os.OpenFile(filename+".torrent", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0640)
	if err != nil {
		return xerrors.Errorf("opening output file: %w", err)
	}
	defer f.Close()
	if _, err := io.Copy(f, out.Body); err != nil {
		return xerrors.Errorf("copying torrent: %w", err)
	}
	if err := f.Close(); err != nil {
		return xerrors.Errorf("closing torrent file: %w", f.Close())
	}
	return nil
}

func serveTorrent(torrentFile string, l net.Listener) error {
	cfg := torrent.NewDefaultClientConfig()
	cfg.Debug = true
	cl, err := torrent.NewClient(cfg)
	if err != nil {
		return xerrors.Errorf("creating torrent client: %w", err)
	}
	defer cl.Close()
	tor, err := cl.AddTorrentFromFile(torrentFile)
	if err != nil {
		return xerrors.Errorf("adding torrent to client: %w", err)
	}
	// This can be expensive for end-users, but is helpful when testing when file names are used but the data changes.
	//tor.VerifyData()
	http.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		r := tor.NewReader()
		defer r.Close()
		http.ServeContent(w, req, tor.Info().Name, time.Time{}, r)
	})
	log.Print("starting http server")
	return http.Serve(l, nil)
}

func viewTorrent(torrentFile string) error {
	l, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return xerrors.Errorf("creating http server listener: %w", err)
	}
	defer l.Close()
	serveErr := make(chan error)
	go func() { serveErr <- serveTorrent(torrentFile, l) }()
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
