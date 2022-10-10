// Copyright © 2022 Bjørn Erik Pedersen <bjorn.erik.pedersen@gmail.com>.
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package lib

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/bep/helpers/envhelpers"
	"github.com/peterbourgon/ff/v3"
	"gopkg.in/yaml.v2"
)

var errUnsupportedFlagType = errors.New("unsupported flag type")

// Parse the flags in the flag set from the provided (presumably commandline)
// args. Additional flags may be provided to parse from a config file and/or
// environment variables in that priority order.
// The Config needs to be initialized with Init before it's used.
func ConfigFromArgs(args []string) (*Config, error) {
	fs := flag.NewFlagSet("s3deploy", flag.ContinueOnError)
	cfg := flagsToConfig(fs)

	if err := ff.Parse(fs, args,
		ff.WithEnvVarPrefix("S3DEPLOY"),
		ff.WithConfigFileFlag("config"),
		ff.WithConfigFileParser(parserYAMLConfig),
		ff.WithAllowMissingConfigFile(true),
	); err != nil {
		return nil, err
	}

	return cfg, nil

}

// Config configures a deployment.
type Config struct {
	fileConf fileConfig

	AccessKey string
	SecretKey string

	SourcePath string
	BucketName string

	// To have multiple sites in one bucket.
	BucketPath string
	RegionName string

	// When set, will invalidate the CDN cache(s) for the updated files.
	CDNDistributionIDs Strings

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
	Ignore          string
	IgnoreRE        *regexp.Regexp // compiled version of Ignore

	// CLI state
	PrintVersion bool

	// Print help
	Help bool

	// Mostly useful for testing.
	baseStore remoteStore

	fs *flag.FlagSet

	initOnce sync.Once
}

func (cfg *Config) Usage() {
	cfg.fs.Usage()
}

func (cfg *Config) Init() error {
	var err error
	cfg.initOnce.Do(func() {
		err = cfg.init()
	})
	return err
}

func (cfg *Config) loadFileConfig() error {
	configFile := cfg.ConfigFile

	if configFile == "" {
		return nil
	}

	data, err := os.ReadFile(configFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	s := envhelpers.Expand(string(data), func(k string) string {
		return os.Getenv(k)
	})
	data = []byte(s)

	conf := fileConfig{}

	err = yaml.Unmarshal(data, &conf)
	if err != nil {
		return err
	}

	for _, r := range conf.Routes {
		r.routerRE, err = regexp.Compile(r.Route)

		if err != nil {
			return err
		}
	}

	cfg.fileConf = conf

	return nil
}

func (cfg *Config) shouldIgnoreLocal(key string) bool {
	if cfg.Ignore == "" {
		return false
	}

	return cfg.IgnoreRE.MatchString(key)
}

func (cfg *Config) shouldIgnoreRemote(key string) bool {
	sub := key[len(cfg.BucketPath):]
	sub = strings.TrimPrefix(sub, "/")

	for _, r := range cfg.fileConf.Routes {
		if r.Ignore && r.routerRE.MatchString(sub) {
			return true
		}
	}

	if cfg.Ignore == "" {
		return false
	}

	return cfg.IgnoreRE.MatchString(sub)
}

func (cfg *Config) init() error {
	if cfg.BucketName == "" {
		return errors.New("AWS bucket is required")
	}

	// The region may be possible for the AWS SDK to figure out from the context.

	if cfg.AccessKey == "" {
		cfg.AccessKey = os.Getenv("AWS_ACCESS_KEY_ID")
	}
	if cfg.SecretKey == "" {
		cfg.SecretKey = os.Getenv("AWS_SECRET_ACCESS_KEY")
	}

	if cfg.AccessKey == "" && cfg.SecretKey == "" {
		// The AWS SDK will fall back to other ways of finding credentials, so we cannot throw ane error here; it will eventually fail.
	} else if cfg.AccessKey == "" || cfg.SecretKey == "" {
		return errors.New("both AWS access key and secret key must be provided")
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

	if cfg.Ignore != "" {
		re, err := regexp.Compile(cfg.Ignore)
		if err != nil {
			return errors.New("cannot compile 'ignore' flag pattern " + err.Error())
		}
		cfg.IgnoreRE = re
	}

	// load additional config (routes) from file if it exists.
	err := cfg.loadFileConfig()
	if err != nil {
		return fmt.Errorf("failed to load config from %s: %s", cfg.ConfigFile, err)
	}

	return nil
}

type Strings []string

func (i *Strings) String() string {
	return strings.Join(*i, ",")
}

func (i *Strings) Set(value string) error {
	*i = append(*i, value)
	return nil
}

func flagsToConfig(f *flag.FlagSet) *Config {
	cfg := &Config{}
	cfg.fs = f
	f.StringVar(&cfg.AccessKey, "key", "", "access key ID for AWS")
	f.StringVar(&cfg.SecretKey, "secret", "", "secret access key for AWS")
	f.StringVar(&cfg.RegionName, "region", "", "name of AWS region")
	f.StringVar(&cfg.BucketName, "bucket", "", "destination bucket name on AWS")
	f.StringVar(&cfg.BucketPath, "path", "", "optional bucket sub path")
	f.StringVar(&cfg.SourcePath, "source", ".", "path of files to upload")
	f.Var(&cfg.CDNDistributionIDs, "distribution-id", "optional CDN distribution ID for cache invalidation, repeat flag for multiple distributions")
	f.StringVar(&cfg.ConfigFile, "config", ".s3deploy.yml", "optional config file")
	f.IntVar(&cfg.MaxDelete, "max-delete", 256, "maximum number of files to delete per deploy")
	f.BoolVar(&cfg.PublicReadACL, "public-access", false, "DEPRECATED: please set -acl='public-read'")
	f.StringVar(&cfg.ACL, "acl", "", "provide an ACL for uploaded objects. to make objects public, set to 'public-read'. all possible values are listed here: https://docs.aws.amazon.com/AmazonS3/latest/userguide/acl-overview.html#canned-acl (default \"private\")")
	f.BoolVar(&cfg.Force, "force", false, "upload even if the etags match")
	f.StringVar(&cfg.Ignore, "ignore", "", "regexp pattern for ignoring files")
	f.BoolVar(&cfg.Try, "try", false, "trial run, no remote updates")
	f.BoolVar(&cfg.Verbose, "v", false, "enable verbose logging")
	f.BoolVar(&cfg.Silent, "quiet", false, "enable silent mode")
	f.BoolVar(&cfg.PrintVersion, "V", false, "print version and exit")
	f.IntVar(&cfg.NumberOfWorkers, "workers", -1, "number of workers to upload files")
	f.BoolVar(&cfg.Help, "h", false, "help")

	return cfg
}

// parserYAMLConfig is a parser for YAML file format. Flags and their values are read
// from the key/value pairs defined in the config file.
// YAML types that cannot easily be represented as a string gets skipped (e.g. maps).
// This is based on https://github.com/peterbourgon/ff/blob/main/ffyaml/ffyaml.go
func parserYAMLConfig(r io.Reader, set func(name, value string) error) error {
	// We need to buffer the Reader so we can expand any environment variables.
	var b bytes.Buffer
	if _, err := io.Copy(&b, r); err != nil {
		return err
	}

	s := envhelpers.Expand(b.String(), func(k string) string {
		return os.Getenv(k)
	})

	r = strings.NewReader(s)

	var m map[string]interface{}
	d := yaml.NewDecoder(r)
	if err := d.Decode(&m); err != nil && err != io.EOF {
		return err
	}
	for key, val := range m {
		values, err := valsToStrs(val)
		if err != nil {
			if err == errUnsupportedFlagType {
				continue
			}
			return err
		}
		for _, value := range values {
			if err := set(key, value); err != nil {
				return err
			}
		}
	}
	return nil
}

func valToStr(val interface{}) (string, error) {
	switch v := val.(type) {
	case byte:
		return string([]byte{v}), nil
	case string:
		return v, nil
	case bool:
		return strconv.FormatBool(v), nil
	case uint64:
		return strconv.FormatUint(v, 10), nil
	case int:
		return strconv.Itoa(v), nil
	case int64:
		return strconv.FormatInt(v, 10), nil
	case float64:
		return strconv.FormatFloat(v, 'g', -1, 64), nil
	case nil:
		return "", nil
	default:
		return "", errUnsupportedFlagType
	}
}

func valsToStrs(val interface{}) ([]string, error) {
	if vals, ok := val.([]interface{}); ok {
		ss := make([]string, len(vals))
		for i := range vals {
			s, err := valToStr(vals[i])
			if err != nil {
				return nil, err
			}
			ss[i] = s
		}
		return ss, nil
	}
	s, err := valToStr(val)
	if err != nil {
		return nil, err
	}
	return []string{s}, nil

}
