// Copyright 2016-present Bjørn Erik Pedersen <bjorn.erik.pedersen@gmail.com>
//
// Portions copyright 2015, Nathan Youngman. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/bep/s3deploy/lib"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {

	// Usage example:
	// s3deploy -source=public/ -bucket=origin.edmontongo.org -key=$AWS_ACCESS_KEY_ID -secret=$AWS_SECRET_ACCESS_KEY

	cfg, err := lib.FlagsToConfig()
	if err != nil {
		log.Fatal(err)
	}

	flag.Parse()

	fmt.Printf("s3deploy %v, commit %v, built at %v\n", version, commit, date)

	if cfg.PrintVersion {
		return
	}

	if cfg.Help {
		flag.Usage()
		return
	}

	stats, err := lib.Deploy(cfg)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(stats.Summary())

}
