package main

import (
	"crypto/md5"
	"flag"
	"fmt"
	"io"
	"log"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/mitchellh/goamz/aws"
	"github.com/mitchellh/goamz/s3"
)

type file struct {
	path         string
	absPath      string
	size         int64
	lastModified time.Time
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

func plan(sourcePath string, destBucket *s3.Bucket, uploadFiles chan<- file) {
	contents, err := destBucket.GetBucketContents()
	if err != nil {
		log.Fatal(err)
	}
	remoteFiles := *contents

	localFiles := make(chan file)
	go walk(sourcePath, localFiles)

	for f := range localFiles {
		up := true
		reason := "not found"

		if key, ok := remoteFiles[f.path]; ok {
			up, reason = shouldOverwrite(f, key)
			// remove from map, whatever is leftover should be deleted:
			delete(remoteFiles, f.path)
		}

		if up {
			fmt.Printf("%s %s, uploading.\n", f.path, reason)
			uploadFiles <- f
		} else {
			fmt.Printf("%s %s, skipping.\n", f.path, reason)
		}
	}
	close(uploadFiles)

	var filesToDelete = make([]string, 0, len(remoteFiles))
	for key := range remoteFiles {
		fmt.Printf("%s not found in source, deleting.\n", key)
		filesToDelete = append(filesToDelete, key)
	}
	cleanup(filesToDelete, destBucket)
}

// can use size and time (or possibly md5) to determine what needs to upload
func shouldOverwrite(source file, dest s3.Key) (bool, string) {
	if source.size != dest.Size {
		return true, "file size mismatch"
	}

	sourceMod := source.lastModified.UTC().Format(time.RFC3339)
	if sourceMod == dest.LastModified {
		// no need to upload if times match, but different times may just be drift
		return false, "last modified match"
	}

	etag, err := calculateETag(source.absPath)
	if err != nil {
		log.Fatalf("Error calculating ETag for %s: ", source.absPath, err)
	}
	if dest.ETag == etag {
		return false, "etags match"
	}
	return true, "etags mismatch"
}

func calculateETag(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := md5.New()
	_, err = io.Copy(h, f)
	if err != nil {
		return "", err
	}
	etag := fmt.Sprintf("\"%x\"", h.Sum(nil))
	return etag, nil
}

func upload(source file, destBucket *s3.Bucket, acl s3.ACL) error {
	f, err := os.Open(source.absPath)
	if err != nil {
		return err
	}
	defer f.Close()

	contentType := mime.TypeByExtension(filepath.Ext(source.path))
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	return destBucket.PutReader(source.path, f, source.size, contentType, acl)
}

func cleanup(paths []string, destBucket *s3.Bucket) error {
	// only can delete 1000 at a time, TODO: split if needed
	return destBucket.MultiDel(paths)
}

var wg sync.WaitGroup

// worker uploads files
func worker(filesToUpload <-chan file, destBucket *s3.Bucket, acl s3.ACL) {
	for f := range filesToUpload {
		err := upload(f, destBucket, acl)
		if err != nil {
			fmt.Printf("Error uploading %s: %s\n", f.path, err)
		}
	}

	wg.Done()
}

func main() {
	var accessKey, secretKey, sourcePath, regionName, bucketName, acl string
	var numberOfWorkers int
	var help bool
	flag.StringVar(&accessKey, "key", "", "Access Key ID for AWS")
	flag.StringVar(&secretKey, "secret", "", "Secret Access Key for AWS")
	flag.StringVar(&regionName, "region", "us-east-1", "Name of region for AWS")
	flag.StringVar(&bucketName, "bucket", "", "Destination bucket name on AWS")
	flag.StringVar(&sourcePath, "source", ".", "path of files to upload")
	flag.StringVar(&acl, "acl", "public-read", "ACL permissions for files uploaded (public-read, private)")
	flag.IntVar(&numberOfWorkers, "workers", 10, "number of workers to upload files")
	flag.BoolVar(&help, "h", false, "help")
	flag.Parse()

	if help {
		flag.Usage()
		return
	}

	if accessKey == "" || secretKey == "" {
		fmt.Println("AWS key and secret are required.")
		flag.Usage()
		os.Exit(1)
	}

	if bucketName == "" {
		fmt.Println("AWS bucket is required.")
		flag.Usage()
		os.Exit(1)
	}

	auth := aws.Auth{AccessKey: accessKey, SecretKey: secretKey}
	region := aws.Regions[regionName]
	s := s3.New(auth, region)
	b := s.Bucket(bucketName)

	filesToUpload := make(chan file)
	for i := 0; i < numberOfWorkers; i++ {
		wg.Add(1)
		go worker(filesToUpload, b, s3.ACL(acl))
	}
	plan(sourcePath, b, filesToUpload)
	wg.Wait()
}
