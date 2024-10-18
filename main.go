// Copyright © 2022 Bjørn Erik Pedersen <bjorn.erik.pedersen@gmail.com>.
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"log"
	"os"
	"runtime/debug"

	"github.com/bep/s3deploy/v2/lib"
)

var (
	commit = "none"
	tag    = "(devel)"
	date   = "unknown"
)

func main() {
	log.SetFlags(0)

	if err := parseAndRun(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

func parseAndRun(args []string) error {
	cfg, err := lib.ConfigFromArgs(args)
	if err != nil {
		return err
	}

	initVersionInfo()

	if !cfg.Silent {
		fmt.Printf("s3deploy %v, commit %v, built at %v\n", tag, commit, date)
	}

	if cfg.Help {
		cfg.Usage()
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
