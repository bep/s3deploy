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
	return flagsToConfig(flag.CommandLine)
}

func flagsToConfig(f *flag.FlagSet) (*Config, error) {
	var cfg Config

	f.StringVar(&cfg.AccessKey, "key", "", "access key ID for AWS")
	f.StringVar(&cfg.SecretKey, "secret", "", "secret access key for AWS")
	f.StringVar(&cfg.RegionName, "region", "", "name of AWS region")
	f.StringVar(&cfg.BucketName, "bucket", "", "destination bucket name on AWS")
	f.StringVar(&cfg.BucketPath, "path", "", "optional bucket sub path")
	f.StringVar(&cfg.SourcePath, "source", ".", "path of files to upload")
	f.StringVar(&cfg.ConfigFile, "config", ".s3deploy.yml", "optional config file")
	f.IntVar(&cfg.MaxDelete, "max-delete", 256, "maximum number of files to delete per deploy")
	f.BoolVar(&cfg.Force, "force", false, "upload even if the etags match")
	f.BoolVar(&cfg.Try, "try", false, "trial run, no remote updates")
	f.BoolVar(&cfg.Verbose, "v", false, "enable verbose logging")
	f.BoolVar(&cfg.Silent, "quiet", false, "enable silent mode")
	f.BoolVar(&cfg.PrintVersion, "V", false, "print version and exit")
	f.IntVar(&cfg.NumberOfWorkers, "workers", -1, "number of workers to upload files")
	f.BoolVar(&cfg.Help, "h", false, "help")

	return &cfg, nil
}

func (cfg *Config) check() error {

	// Generate error messages for required command flags
	if cfg.BucketName == "" {
		return errors.New("AWS bucket is required")
	}
	if cfg.RegionName == "" {
		return errors.New("AWS region is required")
	}
	if cfg.SecretKey == "" {
		return errors.New("AWS secret key is required")
	}
	if cfg.AccessKey == "" {
		return errors.New("AWS access key is required")
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
