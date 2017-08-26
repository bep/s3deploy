// Copyright 2016-present Bjørn Erik Pedersen <bjorn.erik.pedersen@gmail.com>
//
// Portions copyright 2015, Nathan Youngman. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"compress/gzip"
	"crypto/md5"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"mime"
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

type file struct {
	path         string // relative path
	absPath      string // absolute path
	size         int64
	lastModified time.Time
}

// read config from .s3deploy.yml if found.
type config struct {
	Routes []*route `yaml:"routes"`
}

type route struct {
	Route   string            `yaml:"route"`
	Headers map[string]string `yaml:"headers"`
	Gzip    bool              `yaml:"gzip"`

	routerRE *regexp.Regexp // compiled version of Route
}

type deployer struct {
	wg   sync.WaitGroup
	conf *config

	sourcePath string
	bucketName string

	printVersion bool

	// To have multiple sites in one bucket.
	bucketPath string

	regionName string

	// Optional configFile
	configFile string

	verbose bool
	force   bool
}

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	var (
		accessKey, secretKey string
		numberOfWorkers      int

		help bool
	)

	d := &deployer{}

	// Usage example:
	// s3deploy -source=public/ -bucket=origin.edmontongo.org -key=$AWS_ACCESS_KEY_ID -secret=$AWS_SECRET_ACCESS_KEY

	flag.StringVar(&accessKey, "key", "", "Access Key ID for AWS")
	flag.StringVar(&secretKey, "secret", "", "Secret Access Key for AWS")
	flag.StringVar(&d.regionName, "region", "us-east-1", "Name of region for AWS")
	flag.StringVar(&d.bucketName, "bucket", "", "Destination bucket name on AWS")
	flag.StringVar(&d.bucketPath, "path", "", "Optional bucket sub path")
	flag.StringVar(&d.sourcePath, "source", ".", "path of files to upload")
	flag.StringVar(&d.configFile, "config", ".s3deploy.yml", "optional config file")
	flag.BoolVar(&d.force, "force", false, "upload even if the etags match")
	flag.BoolVar(&d.verbose, "v", false, "enable verbose logging")
	flag.BoolVar(&d.printVersion, "V", false, "print version and exit")
	flag.IntVar(&numberOfWorkers, "workers", -1, "number of workers to upload files")
	flag.BoolVar(&help, "h", false, "help")

	flag.Parse()

	fmt.Printf("s3deploy %v, commit %v, built at %v\n", version, commit, date)

	if d.printVersion {
		return
	}

	// load additional config from file if it exists
	err := d.loadConfig()

	if err != nil {
		fmt.Printf("Failed to load config from %s: %s", d.configFile, err)
		os.Exit(-1)
	}

	if help {
		flag.Usage()
		return
	}

	// set to max numb of workers
	if numberOfWorkers == -1 {
		numberOfWorkers = runtime.NumCPU()
	}

	runtime.GOMAXPROCS(numberOfWorkers)

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
		fmt.Println("AWS key and secret are required.")
		flag.Usage()
		os.Exit(1)
	} else {
		// load credentials from file
		var err error
		auth, err = aws.SharedAuth()
		if err != nil {
			fmt.Println("Credentials not found in ~/.aws/credentials")
			flag.Usage()
			os.Exit(1)
		}
	}

	if d.bucketName == "" {
		fmt.Println("AWS bucket is required.")
		flag.Usage()
		os.Exit(1)
	}

	// Get bucket with bucketName

	region := aws.Regions[d.regionName]
	s := s3.New(auth, region)
	b := s.Bucket(d.bucketName)

	filesToUpload := make(chan file)
	errs := make(chan error, 1)
	for i := 0; i < numberOfWorkers; i++ {
		d.wg.Add(1)
		go d.worker(filesToUpload, b, errs)
	}

	d.plan(b, filesToUpload)

	d.wg.Wait()

	// if any errors occurred during upload, exit with an error status code
	select {
	case <-errs:
		fmt.Println("Errors occurred while uploading files.")
		os.Exit(1)
	default:
	}
}

// plan figures out which files need to be uploaded.
func (d *deployer) plan(destBucket *s3.Bucket, uploadFiles chan<- file) {
	// List all files in the remote bucket
	contents, err := destBucket.GetBucketContents()
	if err != nil {
		log.Fatal(err)
	}
	remoteFiles := *contents

	// All local files at sourcePath
	localFiles := make(chan file)
	go walk(d.sourcePath, localFiles)

	for f := range localFiles {
		// default: upload because local file not found on remote.
		up := true
		reason := "not found"

		bucketPath := filepath.ToSlash(f.path)

		if d.bucketPath != "" {
			bucketPath = path.Join(d.bucketPath, bucketPath)
		}

		if key, ok := remoteFiles[bucketPath]; ok {
			up, reason = d.shouldOverwrite(f, key)
			if d.force {
				up = true
				reason = "force"
			}

			// remove from map, whatever is leftover should be deleted:
			delete(remoteFiles, bucketPath)
		}

		if up {
			fmt.Printf("%s %s, uploading.\n", f.path, reason)
			uploadFiles <- f
		} else if d.verbose {
			fmt.Printf("%s %s, skipping.\n", f.path, reason)
		}
	}
	close(uploadFiles)

	// any remote files not found locally should be removed:
	var filesToDelete = make([]string, 0, len(remoteFiles))
	for key := range remoteFiles {
		if !strings.HasPrefix(key, d.bucketPath) {
			// Not part of this site: Keep!
			continue
		}
		fmt.Printf("%s not found in source, deleting.\n", key)
		filesToDelete = append(filesToDelete, key)
	}
	cleanup(filesToDelete, destBucket)
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
			files <- file{path: rel, absPath: abs, size: info.Size(), lastModified: info.ModTime()}
		}
		return nil
	})
	close(files)
}

// shouldOverwrite uses size or md5 to determine what needs to be uploaded
func (d *deployer) shouldOverwrite(source file, dest s3.Key) (up bool, reason string) {

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
func (d *deployer) worker(filesToUpload <-chan file, destBucket *s3.Bucket, errs chan<- error) {
	for f := range filesToUpload {
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

	d.wg.Done()
}

func (d *deployer) upload(source file, destBucket *s3.Bucket) error {

	f, err := os.Open(source.absPath)
	if err != nil {
		return err
	}
	defer f.Close()

	contentType := mime.TypeByExtension(filepath.Ext(source.path))
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	headers := map[string][]string{
		"Content-Type": {contentType},
	}

	route, err := d.findRoute(source.path)

	if err != nil {
		return err
	}

	var (
		r    io.Reader = f
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

	if d.bucketPath != "" {
		bucketPath = path.Join(d.bucketPath, bucketPath)
	}

	return destBucket.PutReaderHeader(bucketPath, r, size, headers, "public-read")
}

func (d *deployer) findRoute(path string) (*route, error) {
	if d.conf == nil {
		return nil, nil
	}

	var err error

	for _, r := range d.conf.Routes {
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

func (d *deployer) loadConfig() error {

	configFile := d.configFile

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

	conf := &config{}

	err = yaml.Unmarshal(data, conf)
	if err != nil {
		return err
	}

	d.conf = conf

	return nil
}
