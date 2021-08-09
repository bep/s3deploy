// Copyright © 2018 Bjørn Erik Pedersen <bjorn.erik.pedersen@gmail.com>.
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package lib

import (
	"errors"
	"flag"
	"log"
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

	// When set, will invalidate the CDN cache for the updated files.
	CDNDistributionID string

	// Optional configFile
	ConfigFile string

	NumberOfWorkers int
	MaxDelete       int
	ACL             string
	PublicReadACL   bool
	Verbose         bool
	Silent          bool
	Force           bool
	Try             bool

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
	f.StringVar(&cfg.CDNDistributionID, "distribution-id", "", "optional CDN distribution ID for cache invalidation")
	f.StringVar(&cfg.ConfigFile, "config", ".s3deploy.yml", "optional config file")
	f.IntVar(&cfg.MaxDelete, "max-delete", 256, "maximum number of files to delete per deploy")
	f.BoolVar(&cfg.PublicReadACL, "public-access", false, "DEPRECATED: please set -acl='public-read'")
	f.StringVar(&cfg.ACL, "acl", "", "provide an ACL for uploaded objects. to make objects public, set to 'public-read'. all possible values are listed here: https://docs.aws.amazon.com/AmazonS3/latest/userguide/acl-overview.html#canned-acl (default \"private\")")
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

	if cfg.PublicReadACL {
		log.Print("WARNING: the 'public-access' flag is deprecated. Please use -acl='public-read' instead.")
	}

	if cfg.PublicReadACL && cfg.ACL != "" {
		return errors.New("you passed a value for the flags public-access and acl, which is not supported. the public-access flag is deprecated. please use the acl flag moving forward")
	}

	return nil
}
