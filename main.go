// Copyright © 2022 Bjørn Erik Pedersen <bjorn.erik.pedersen@gmail.com>.
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"flag"
	"fmt"
	"log"
	"runtime/debug"

	"github.com/bep/s3deploy/v2/lib"
)

var (
	version = "v2"
	commit  = "none"
	date    = "unknown"
)

func main() {
	log.SetFlags(0)

	// Use:
	// s3deploy -source=public/ -bucket=example.com -region=eu-west-1 -key=$AWS_ACCESS_KEY_ID -secret=$AWS_SECRET_ACCESS_KEY
	if err := parseAndRun(); err != nil {
		log.Fatal(err)
	}
}

func parseAndRun() error {
	cfg, err := lib.FlagsToConfig()
	if err != nil {
		return err
	}

	flag.Parse()

	initVersionInfo()

	if !cfg.Silent {
		fmt.Printf("s3deploy %v, commit %v, built at %v\n", version, commit, date)
	}

	if cfg.Help {
		flag.Usage()
		return nil
	}

	if cfg.PrintVersion {
		return nil
	}

	stats, err := lib.Deploy(cfg)
	if err != nil {
		return err
	}

	if !cfg.Silent {
		fmt.Println(stats.Summary())
	}

	return nil

}

func initVersionInfo() {

	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return
	}

	version = bi.Main.Version

	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs":
		case "vcs.revision":
			commit = s.Value
		case "vcs.time":
			date = s.Value
		case "vcs.modified":
		}
	}

}
