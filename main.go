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
	var (
		printVersion bool

		help bool
	)

	var cfg lib.Config

	// Usage example:
	// s3deploy -source=public/ -bucket=origin.edmontongo.org -key=$AWS_ACCESS_KEY_ID -secret=$AWS_SECRET_ACCESS_KEY

	flag.StringVar(&cfg.AccessKey, "key", "", "Access Key ID for AWS")
	flag.StringVar(&cfg.SecretKey, "secret", "", "Secret Access Key for AWS")
	flag.StringVar(&cfg.RegionName, "region", "us-east-1", "Name of region for AWS")
	flag.StringVar(&cfg.BucketName, "bucket", "", "Destination bucket name on AWS")
	flag.StringVar(&cfg.BucketPath, "path", "", "Optional bucket sub path")
	flag.StringVar(&cfg.SourcePath, "source", ".", "path of files to upload")
	flag.StringVar(&cfg.ConfigFile, "config", ".s3deploy.yml", "optional config file")
	flag.BoolVar(&cfg.Force, "force", false, "upload even if the etags match")
	flag.BoolVar(&cfg.Verbose, "v", false, "enable verbose logging")
	flag.BoolVar(&printVersion, "V", false, "print version and exit")
	flag.IntVar(&cfg.NumberOfWorkers, "workers", -1, "number of workers to upload files")
	flag.BoolVar(&help, "h", false, "help")

	flag.Parse()

	fmt.Printf("s3deploy %v, commit %v, built at %v\n", version, commit, date)

	if printVersion {
		return
	}

	if help {
		flag.Usage()
		return
	}

	if err := lib.Deploy(cfg); err != nil {
		log.Fatal(err)
	}
}
