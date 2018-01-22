// Copyright 2016-present Bjørn Erik Pedersen <bjorn.erik.pedersen@gmail.com>
//
// Portions copyright 2015, Nathan Youngman. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package lib

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/md5"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/mitchellh/goamz/aws"
	"github.com/mitchellh/goamz/s3"
	"gopkg.in/yaml.v2"
)

type Deployer struct {
	wg  sync.WaitGroup
	cfg *Config
}

type Config struct {
	conf *fileConfig

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

	Verbose bool
	Force   bool

	// CLI state
	PrintVersion bool

	Help bool
}

type DeployStats struct {
	Deleted  []string
	Uploaded []string
	Skipped  []string
}

func (d DeployStats) Summary() string {
	return fmt.Sprintf("Deleted: %d Uploaded: %d Skipped: %d (%.0f%% changed)", len(d.Deleted), len(d.Uploaded), len(d.Skipped), d.PercentageChanged())
}

func (d DeployStats) FileCountChanged() int {
	return len(d.Deleted) + len(d.Uploaded)
}

func (d DeployStats) FileCount() int {
	return d.FileCountChanged() + len(d.Skipped)
}

func (d DeployStats) PercentageChanged() float32 {
	if d.FileCount() == 0 {
		return 0.0
	}
	return (float32(d.FileCountChanged()) / float32(d.FileCount()) * 100)
}

func (d DeployStats) AllChanged() []string {
	return append(d.Deleted, d.Uploaded...)
}

type file struct {
	path         string // relative path
	absPath      string // absolute path
	size         int64
	lastModified time.Time
}

// read config from .s3deploy.yml if found.
type fileConfig struct {
	Routes []*route `yaml:"routes"`
}

type route struct {
	Route   string            `yaml:"route"`
	Headers map[string]string `yaml:"headers"`
	Gzip    bool              `yaml:"gzip"`

	routerRE *regexp.Regexp // compiled version of Route
}

// Reads command-line flags from os.Args[1:] into Config.
// Note that flag.Parse is not called.
func FlagsToConfig() (*Config, error) {
	var cfg Config

	flag.StringVar(&cfg.AccessKey, "key", "", "Access Key ID for AWS")
	flag.StringVar(&cfg.SecretKey, "secret", "", "Secret Access Key for AWS")
	flag.StringVar(&cfg.RegionName, "region", "us-east-1", "Name of region for AWS")
	flag.StringVar(&cfg.BucketName, "bucket", "", "Destination bucket name on AWS")
	flag.StringVar(&cfg.BucketPath, "path", "", "Optional bucket sub path")
	flag.StringVar(&cfg.SourcePath, "source", ".", "path of files to upload")
	flag.StringVar(&cfg.ConfigFile, "config", ".s3deploy.yml", "optional config file")
	flag.BoolVar(&cfg.Force, "force", false, "upload even if the etags match")
	flag.BoolVar(&cfg.Verbose, "v", false, "enable verbose logging")
	flag.BoolVar(&cfg.PrintVersion, "V", false, "print version and exit")
	flag.IntVar(&cfg.NumberOfWorkers, "workers", -1, "number of workers to upload files")
	flag.BoolVar(&cfg.Help, "h", false, "help")

	return &cfg, nil

}

func Deploy(cfg *Config) (DeployStats, error) {
	var (
		d               = &Deployer{cfg: cfg}
		numberOfWorkers = cfg.NumberOfWorkers
		accessKey       = cfg.AccessKey
		secretKey       = cfg.SecretKey
		stats           DeployStats
	)

	// set to max numb of workers
	if numberOfWorkers == -1 {
		numberOfWorkers = runtime.NumCPU()
	}

	runtime.GOMAXPROCS(numberOfWorkers)

	// load additional config from file if it exists
	err := d.loadConfig()

	if err != nil {
		return stats, fmt.Errorf("Failed to load config from %s: %s", cfg.ConfigFile, err)
	}

	var auth aws.Auth

	if accessKey == "" {
		accessKey = os.Getenv("AWS_ACCESS_KEY_ID")
	}

	if secretKey == "" {
		secretKey = os.Getenv("AWS_SECRET_ACCESS_KEY")
	}

	if accessKey != "" && secretKey != "" {
		auth = aws.Auth{AccessKey: accessKey, SecretKey: secretKey}
	} else if accessKey != "" || secretKey != "" {
		// provided one but not both
		return stats, errors.New("AWS key and secret are required.")
	} else {
		// load credentials from file
		var err error
		auth, err = aws.SharedAuth()
		if err != nil {
			return stats, errors.New("Credentials not found in ~/.aws/credentials")
		}
	}

	if d.cfg.BucketName == "" {
		return stats, errors.New("AWS bucket is required.")
	}

	// Get bucket with bucketName

	region := aws.Regions[d.cfg.RegionName]
	s := s3.New(auth, region)
	b := s.Bucket(d.cfg.BucketName)

	filesToUpload := make(chan file)
	quit := make(chan struct{})

	errs := make(chan error, 1)
	for i := 0; i < numberOfWorkers; i++ {
		d.wg.Add(1)
		go d.worker(filesToUpload, b, errs, quit)
	}

	stats, err = d.plan(b, filesToUpload)
	if err != nil {
		close(quit)
	}

	d.wg.Wait()

	// if any errors occurred during upload, exit with an error status code
	select {
	case <-errs:
		return stats, errors.New("Errors occurred while uploading files.")
	default:
		return stats, nil
	}

}

// plan figures out which files need to be uploaded.
func (d *Deployer) plan(destBucket *s3.Bucket, uploadFiles chan<- file) (DeployStats, error) {
	var stats DeployStats
	// List all files in the remote bucket
	contents, err := destBucket.GetBucketContents()
	if err != nil {
		return stats, err
	}
	remoteFiles := *contents

	// All local files at sourcePath
	localFiles := make(chan file)
	go walk(d.cfg.SourcePath, localFiles)

	for f := range localFiles {
		// default: upload because local file not found on remote.
		up := true
		reason := "not found"

		bucketPath := f.path

		if d.cfg.BucketPath != "" {
			bucketPath = path.Join(d.cfg.BucketPath, bucketPath)
		}

		if key, ok := remoteFiles[bucketPath]; ok {
			up, reason = d.shouldOverwrite(f, key)
			if d.cfg.Force {
				up = true
				reason = "force"
			}

			// remove from map, whatever is leftover should be deleted:
			delete(remoteFiles, bucketPath)
		}

		if up {
			fmt.Printf("%s %s, uploading.\n", f.path, reason)
			uploadFiles <- f
			stats.Uploaded = append(stats.Uploaded, f.path)
		} else {
			if d.cfg.Verbose {
				fmt.Printf("%s %s, skipping.\n", f.path, reason)
			}
			stats.Skipped = append(stats.Skipped, f.path)
		}
	}
	close(uploadFiles)

	// any remote files not found locally should be removed:
	var filesToDelete = make([]string, 0, len(remoteFiles))
	for key := range remoteFiles {
		if !strings.HasPrefix(key, d.cfg.BucketPath) {
			// Not part of this site: Keep!
			continue
		}
		fmt.Printf("%s not found in source, deleting.\n", key)
		filesToDelete = append(filesToDelete, key)
		stats.Deleted = append(stats.Deleted, strings.TrimPrefix(key, d.cfg.BucketPath))
	}
	cleanup(filesToDelete, destBucket)

	return stats, nil
}

// walk a local directory
func walk(basePath string, files chan<- file) {
	filepath.Walk(basePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			// skip hidden directories like .git
			if strings.HasPrefix(info.Name(), ".") {
				return filepath.SkipDir
			}
		} else {
			if info.Name() == ".DS_Store" {
				return nil
			}
			abs, err := filepath.Abs(path)
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(basePath, path)
			if err != nil {
				return err
			}
			rel = filepath.ToSlash(rel)
			files <- file{path: rel, absPath: abs, size: info.Size(), lastModified: info.ModTime()}
		}
		return nil
	})
	close(files)
}

// shouldOverwrite uses size or md5 to determine what needs to be uploaded
func (d *Deployer) shouldOverwrite(source file, dest s3.Key) (up bool, reason string) {

	f, err := os.Open(source.absPath)
	if err != nil {
		log.Fatalf("Error opening file %s: %v", source.absPath, err)
	}
	defer f.Close()

	var (
		sourceSize           = source.size
		r          io.Reader = f
	)

	route, err := d.findRoute(source.path)

	if err != nil {
		log.Fatalf("Error finding route for %s: %v", source.absPath, err)
	}

	if route != nil && route.Gzip {
		var b bytes.Buffer
		gz := gzip.NewWriter(&b)
		io.Copy(gz, f)
		gz.Close()
		r = &b
		sourceSize = int64(b.Len())
	}

	if sourceSize != dest.Size {
		return true, "file size mismatch"
	}

	sourceMod := source.lastModified.UTC().Format(time.RFC3339)
	if sourceMod == dest.LastModified {
		// no need to upload if times match, but different times may just be drift
		return false, "last modified match"
	}

	etag, err := calculateETag(r)
	if err != nil {
		log.Fatalf("Error calculating ETag for %s: %v", source.absPath, err)
	}
	if dest.ETag == etag {
		return false, "etags match"
	}
	return true, "etags mismatch"
}

func calculateETag(r io.Reader) (string, error) {

	h := md5.New()

	_, err := io.Copy(h, r)
	if err != nil {
		return "", err
	}
	etag := fmt.Sprintf("\"%x\"", h.Sum(nil))
	return etag, nil
}

func cleanup(paths []string, destBucket *s3.Bucket) error {
	// only can delete 1000 at a time, TODO: split if needed
	return destBucket.MultiDel(paths)
}

// worker uploads files
func (d *Deployer) worker(filesToUpload <-chan file, destBucket *s3.Bucket, errs chan<- error, quit chan struct{}) {
	defer d.wg.Done()
	for {
		select {
		case <-quit:
			return
		case f, ok := <-filesToUpload:
			if !ok {
				return
			}
			err := d.upload(f, destBucket)
			if err != nil {
				fmt.Printf("Error uploading %s: %s\n", f.path, err)
				// if there are no errors on the channel, put this one there
				select {
				case errs <- err:
				default:
				}
			}
		}
	}
}

func (d *Deployer) upload(source file, destBucket *s3.Bucket) error {

	f, err := os.Open(source.absPath)
	if err != nil {
		return err
	}
	defer f.Close()

	br := bufio.NewReader(f)
	const magicSize = 512 // Size that DetectContentType expects
	peek, err := br.Peek(magicSize)
	if err != nil {
		return err
	}

	contentType := http.DetectContentType(peek)

	headers := map[string][]string{
		"Content-Type": {contentType},
	}

	route, err := d.findRoute(source.path)

	if err != nil {
		return err
	}

	var (
		r    io.Reader = br
		size           = source.size
	)

	if route != nil {

		for k, v := range route.Headers {
			headers[k] = []string{v}
		}

		if route.Gzip {
			var b bytes.Buffer
			gz := gzip.NewWriter(&b)
			io.Copy(gz, f)
			gz.Close()
			r = &b
			size = int64(b.Len())
			headers["Content-Encoding"] = []string{"gzip"}
		}
	}

	// TODO(bep)
	bucketPath := filepath.FromSlash(source.path)

	if d.cfg.BucketPath != "" {
		bucketPath = path.Join(d.cfg.BucketPath, bucketPath)
	}

	return destBucket.PutReaderHeader(bucketPath, r, size, headers, "public-read")
}

func (d *Deployer) findRoute(path string) (*route, error) {
	if d.cfg.conf == nil {
		return nil, nil
	}

	var err error

	for _, r := range d.cfg.conf.Routes {
		if r.routerRE == nil {
			r.routerRE, err = regexp.Compile(r.Route)

			if err != nil {
				return nil, err
			}
		}
		if r.routerRE.MatchString(path) {
			return r, nil
		}

	}

	// no route found
	return nil, nil

}

func (d *Deployer) loadConfig() error {

	configFile := d.cfg.ConfigFile

	if configFile == "" {
		return nil
	}

	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		return nil
	}

	data, err := ioutil.ReadFile(configFile)

	if os.IsNotExist(err) {
		return nil
	}

	conf := &fileConfig{}

	err = yaml.Unmarshal(data, conf)
	if err != nil {
		return err
	}

	d.cfg.conf = conf

	return nil
}
