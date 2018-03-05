// Copyright © 2018 Bjørn Erik Pedersen <bjorn.erik.pedersen@gmail.com>.
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package lib

import (
	"flag"
)

// Config configures a deployment.
type Config struct {
	conf fileConfig

	AccessKey string
	SecretKey string

	SourcePath string
	BucketName string

	// To have multiple sites in one bucket.
	BucketPath string
	RegionName string

	// Optional configFile
	ConfigFile string

	NumberOfWorkers int
	MaxDelete       int

	Verbose bool
	Silent  bool
	Force   bool

	// CLI state
	PrintVersion bool

	Help bool

	// Mostly useful for testing.
	store remoteStore
}

// FlagsToConfig reads command-line flags from os.Args[1:] into Config.
// Note that flag.Parse is not called.
func FlagsToConfig() (*Config, error) {
	var cfg Config

	flag.StringVar(&cfg.AccessKey, "key", "", "access key ID for AWS")
	flag.StringVar(&cfg.SecretKey, "secret", "", "secret access key for AWS")
	flag.StringVar(&cfg.RegionName, "region", "", "name of AWS region")
	flag.StringVar(&cfg.BucketName, "bucket", "", "destination bucket name on AWS")
	flag.StringVar(&cfg.BucketPath, "path", "", "optional bucket sub path")
	flag.StringVar(&cfg.SourcePath, "source", ".", "path of files to upload")
	flag.StringVar(&cfg.ConfigFile, "config", ".s3deploy.yml", "optional config file")
	flag.IntVar(&cfg.MaxDelete, "max-delete", 256, "maximum number of files to delete per deploy")
	flag.BoolVar(&cfg.Force, "force", false, "upload even if the etags match")
	flag.BoolVar(&cfg.Verbose, "v", false, "enable verbose logging")
	flag.BoolVar(&cfg.Silent, "quiet", false, "enable silent mode")
	flag.BoolVar(&cfg.PrintVersion, "V", false, "print version and exit")
	flag.IntVar(&cfg.NumberOfWorkers, "workers", -1, "number of workers to upload files")
	flag.BoolVar(&cfg.Help, "h", false, "help")

	return &cfg, nil

}
