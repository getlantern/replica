package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	cli "github.com/jawher/mow.cli"

	"github.com/getlantern/replica"
)

var s3Client = &replica.Client{
	Storage:                &replica.S3Storage{HttpClient: http.DefaultClient},
	Endpoint:               replica.DefaultEndpoint,
	ReplicaServiceEndpoint: &url.URL{Scheme: "https", Host: "replica-search.lantern.io"},
}

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
	app.Command("upload", "uploads a file to S3 and returns the S3 key", uploadToS3)
	app.Command("get-torrent", "retrieve BitTorrent metainfo for a Replica S3 key", getTorrent)
	app.Command("open-torrent", "open torrent contents", openTorrent)
	return app.Run(os.Args)
}

func uploadToS3(cmd *cli.Cmd) {
	uploadTo(cmd, s3Client)
}

func uploadTo(cmd *cli.Cmd, client *replica.Client) {
	file := cmd.StringArg("FILE", "", "file to upload")
	providerID := cmd.StringOpt("p provider-id", "", "Replica content provider and id (eg youtube-IDHERE")
	filename := cmd.StringOpt("n filename", "", "Optional filename to be uploaded as. If not provided, it will use the filename of the specified FILE")
	cmd.Action = func() {
		checkAction(func() error {
			output, err := func() (replica.UploadOutput, error) {
				if *providerID != "" {
					uConfig := &replica.ProviderUploadConfig{
						File:       *file,
						ProviderID: *providerID,
						Name:       *filename,
					}
					return client.UploadFileDirectly(uConfig)
				}
				asName := *filename
				if asName == "" {
					asName = filepath.Base(*file)
				}
				return client.UploadFile(*file, asName)
			}()
			if err != nil {
				return err
			}
			log.Printf("uploaded to %q", output.Upload)
			// This could come from the link response from the upload service if that was exposed.
			fmt.Printf("%s\n", replica.CreateLink(output.HashInfoBytes(), output.Upload, output.FilePath()))
			return nil
		}())
	}
	cmd.Spec = "[-p] [-n] FILE"
}

func getTorrent(cmd *cli.Cmd) {
	name := cmd.StringArg("NAME", "", "Replica S3 object name")
	cmd.Action = func() {
		checkAction(func() error {
			uuid, _ := uuid.Parse(*name)
			obj, err := s3Client.GetObject(replica.UploadPrefix{replica.UUIDPrefix{uuid}}.TorrentKey())
			if err != nil {
				return err
			}
			defer obj.Close()
			_, err = io.Copy(os.Stdout, obj)
			return err
		}())
	}
}

func openTorrent(cmd *cli.Cmd) {
	file := cmd.StringArg("FILE", "", "torrent to open")
	debug := cmd.BoolOpt("debug d", false, "debug torrent client")
	cmd.Action = func() { checkAction(viewTorrent(*file, *debug)) }
}
