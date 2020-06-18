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
	replicaClient := replica.Client{
		Endpoint: replica.DefaultEndpoint,
	}
	app.Command("upload", "uploads a file to S3 and returns the S3 key", func(cmd *cli.Cmd) {
		files := cmd.StringsArg("FILE", nil, "file to upload")
		cmd.Action = func() {
			checkAction(func() error {
				for _, f := range *files {
					output, err := replicaClient.UploadFile(f)
					if err != nil {
						return err
					}
					log.Printf("uploaded to %q", output.Upload)
					fmt.Printf("%s\n", replica.CreateLink(output.HashInfoBytes(), output.Upload, output.FilePath()))
				}
				return nil
			}())
		}
		cmd.Spec = "FILE..."
	})
	app.Command("get-torrent", "retrieve BitTorrent metainfo for a Replica S3 key", func(cmd *cli.Cmd) {
		name := cmd.StringArg("NAME", "", "Replica S3 object name")
		cmd.Action = func() {
			checkAction(func() error {
				uuid, _ := uuid.Parse(*name)
				obj, err := replicaClient.GetObject(replica.UploadPrefix{uuid}.TorrentKey())
				if err != nil {
					return err
				}
				defer obj.Close()
				_, err = io.Copy(os.Stdout, obj)
				return err
			}())
		}
	})
	app.Command("open-torrent", "open torrent contents", func(cmd *cli.Cmd) {
		file := cmd.StringArg("FILE", "", "torrent to open")
		debug := cmd.BoolOpt("debug d", false, "debug torrent client")
		cmd.Action = func() { checkAction(viewTorrent(*file, *debug)) }
	})
	return app.Run(os.Args)
}
