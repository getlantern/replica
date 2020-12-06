package main

import (
	"fmt"
	"io"
	"log"
	"os"

	"github.com/google/uuid"
	cli "github.com/jawher/mow.cli"

	"github.com/getlantern/replica"
)

var s3Client = &replica.Client{replica.NewS3Storage(), replica.DefaultEndpoint}

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
			var uConfig replica.UploadConfig
			if *providerID != "" {
				uConfig = &replica.ProviderUploadConfig{
					File:       *file,
					ProviderID: *providerID,
					Name:       *filename,
				}
			} else {
				uConfig = replica.NewUUIDUploadConfig(*file, *filename)
			}

			output, err := client.UploadFile(uConfig)
			if err != nil {
				return err
			}
			log.Printf("uploaded to %q", output.Upload)
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
