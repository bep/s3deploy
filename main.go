// Copyright © 2018 Bjørn Erik Pedersen <bjorn.erik.pedersen@gmail.com>.
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"flag"
	"fmt"
	"log"

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

	cfg, err := lib.FlagsToConfig()
	if err != nil {
		log.Fatal(err)
	}

	flag.Parse()

	if !cfg.Silent {
		fmt.Printf("s3deploy %v, commit %v, built at %v\n", version, commit, date)
	}

	if cfg.Help {
		flag.Usage()
		return
	}

	if cfg.PrintVersion {
		return
	}

	stats, err := lib.Deploy(cfg)
	if err != nil {
		log.Fatal("error: ", err)
	}

	if !cfg.Silent {
		fmt.Println(stats.Summary())
	}
}
