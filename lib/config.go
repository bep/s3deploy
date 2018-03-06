// Copyright © 2018 Bjørn Erik Pedersen <bjorn.erik.pedersen@gmail.com>.
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package lib

import (
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
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
	Try     bool

	// CLI state
	PrintVersion bool

	Help bool

	// Mostly useful for testing.
	baseStore remoteStore
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
	flag.BoolVar(&cfg.Try, "try", false, "trial run, no remote updates")
	flag.BoolVar(&cfg.Verbose, "v", false, "enable verbose logging")
	flag.BoolVar(&cfg.Silent, "quiet", false, "enable silent mode")
	flag.BoolVar(&cfg.PrintVersion, "V", false, "print version and exit")
	flag.IntVar(&cfg.NumberOfWorkers, "workers", -1, "number of workers to upload files")
	flag.BoolVar(&cfg.Help, "h", false, "help")

	return &cfg, nil

}

func (cfg *Config) check() error {

	if cfg.BucketName == "" {
		return errors.New("AWS bucket is required")
	}

	cfg.SourcePath = filepath.Clean(cfg.SourcePath)

	// Sanity check to prevent people from uploading their entire disk.
	// The returned path from filepath.Clean ends in a slash only if it represents
	// a root directory, such as "/" on Unix or `C:\` on Windows.
	if strings.HasSuffix(cfg.SourcePath, string(os.PathSeparator)) {
		return errors.New("invalid source path: Cannot deploy from root")
	}

	return nil
}
