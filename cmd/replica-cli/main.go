package main

import (
	"fmt"
	"log"
	"os"

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
	app.Command("upload", "uploads a file to S3 and returns the S3 key", func(cmd *cli.Cmd) {
		files := cmd.StringsArg("FILE", nil, "file to upload")
		cmd.Action = func() {
			checkAction(func() error {
				for _, f := range *files {
					output, err := replica.UploadFile(f)
					if err != nil {
						return err
					}
					log.Printf("uploaded to %q", output.S3Prefix)
					fmt.Printf("%s\n", replica.CreateLink(output.Metainfo.HashInfoBytes(), output.S3Prefix, output.Info.Name))
				}
				return nil
			}())
		}
		cmd.Spec = "FILE..."
	})
	app.Command("get-torrent", "retrieve BitTorrent metainfo for a Replica S3 key", func(cmd *cli.Cmd) {
		name := cmd.StringArg("NAME", "", "Replica S3 object name")
		cmd.Action = func() { checkAction(replica.GetTorrent(*name)) }
	})
	app.Command("open-torrent", "open torrent contents", func(cmd *cli.Cmd) {
		file := cmd.StringArg("FILE", "", "torrent to open")
		debug := cmd.BoolOpt("debug d", false, "debug torrent client")
		cmd.Action = func() { checkAction(viewTorrent(*file, *debug)) }
	})
	return app.Run(os.Args)
}
