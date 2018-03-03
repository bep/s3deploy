// Copyright © 2018 Bjørn Erik Pedersen <bjorn.erik.pedersen@gmail.com>.
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package lib

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"gopkg.in/yaml.v2"
)

// Deployer deploys.
type Deployer struct {
	cfg   *Config
	stats *DeployStats

	g *errgroup.Group

	filesToUpload chan *osFile
	filesToDelete []string

	verbosew io.Writer

	store remoteStore
}

type upload struct {
	*osFile
	reason string
}

// Deploy deploys to the remote based on the given config.
func Deploy(cfg *Config) (DeployStats, error) {
	if !cfg.Silent {
		start := time.Now()
		defer func() {
			fmt.Printf("Total in %.2f seconds\n", time.Since(start).Seconds())
		}()

	}

	verbosew := ioutil.Discard
	if cfg.Verbose && !cfg.Silent {
		verbosew = os.Stdout
	}

	var g *errgroup.Group

	ctx, cancel := context.WithCancel(context.Background())
	g, ctx = errgroup.WithContext(ctx)
	defer cancel()

	var d = &Deployer{g: g, verbosew: verbosew, filesToUpload: make(chan *osFile), cfg: cfg, stats: &DeployStats{}}

	if d.cfg.BucketName == "" {
		return *d.stats, errors.New("AWS bucket is required")
	}

	numberOfWorkers := cfg.NumberOfWorkers

	if numberOfWorkers <= 0 {
		numberOfWorkers = runtime.NumCPU()
	}

	// load additional config from file if it exists
	err := d.loadConfig()
	if err != nil {
		return *d.stats, fmt.Errorf("Failed to load config from %s: %s", cfg.ConfigFile, err)
	}

	store := d.cfg.store
	if store == nil {
		store, err = newRemoteStore(*d.cfg)
		if err != nil {
			return *d.stats, err
		}
	}
	d.store = store

	for i := 0; i < numberOfWorkers; i++ {
		g.Go(func() error {
			return d.upload(ctx)
		})
	}

	err = d.plan(ctx)
	if err != nil {
		cancel()
	}

	errg := g.Wait()

	if err != nil {
		return *d.stats, err
	}

	if errg != nil && errg != context.Canceled {
		return *d.stats, errg
	}

	err = d.store.DeleteObjects(
		context.Background(),
		d.filesToDelete,
		withDeleteStats(d.stats),
		withMaxDelete(d.cfg.MaxDelete))

	return *d.stats, err
}

func (d *Deployer) enqueueUpload(ctx context.Context, f *osFile, reason string) {
	fmt.Fprintf(d.verbosew, "%s (%s) uploading …\n", f.relPath, reason)
	select {
	case <-ctx.Done():
	case d.filesToUpload <- f:
	}
}

func (d *Deployer) skipFile(f *osFile) {
	fmt.Fprintf(d.verbosew, "%s skipping …\n", f.relPath)
	d.stats.Skipped++
}

func (d *Deployer) enqueueDelete(key string) {
	fmt.Fprintf(d.verbosew, "%s not found in source, deleting.\n", key)
	d.filesToDelete = append(d.filesToDelete, key)
}

// plan figures out which files need to be uploaded.
func (d *Deployer) plan(ctx context.Context) error {
	remoteFiles, err := d.store.FileMap()
	if err != nil {
		return err
	}

	// All local files at sourcePath
	localFiles := make(chan *osFile)
	d.g.Go(func() error {
		return d.walk(ctx, d.cfg.SourcePath, localFiles)
	})

	for f := range localFiles {
		// default: upload because local file not found on remote.
		up := true
		reason := "not found"

		bucketPath := f.relPath

		if d.cfg.BucketPath != "" {
			bucketPath = path.Join(d.cfg.BucketPath, bucketPath)
		}

		if remoteFile, ok := remoteFiles[bucketPath]; ok {
			if d.cfg.Force {
				up = true
				reason = "force"
			} else {
				up, reason = f.shouldThisReplace(remoteFile)
			}

			// remove from map, whatever is leftover should be deleted:
			delete(remoteFiles, bucketPath)
		}

		if up {
			d.enqueueUpload(ctx, f, reason)
		} else {
			d.skipFile(f)
		}
	}
	close(d.filesToUpload)

	// any remote files not found locally should be removed:
	for key := range remoteFiles {
		if !strings.HasPrefix(key, d.cfg.BucketPath) {
			// Not part of this site: Keep!
			continue
		}
		d.enqueueDelete(key)
	}

	return nil
}

// walk a local directory
func (d *Deployer) walk(ctx context.Context, basePath string, files chan<- *osFile) error {
	err := filepath.Walk(basePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			// skip hidden directories like .git
			if path != basePath && strings.HasPrefix(info.Name(), ".") {
				return filepath.SkipDir
			}

			return nil
		}

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
		f, err := newOSFile(d.cfg.conf.Routes, rel, abs, info)
		if err != nil {
			return err
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case files <- f:
		}

		return nil
	})

	close(files)

	return err
}

func (d *Deployer) upload(ctx context.Context) error {
	for {
		select {
		case f, ok := <-d.filesToUpload:
			if !ok {
				return nil
			}
			err := d.store.Put(ctx, f, withUploadStats(d.stats))
			if err != nil {
				return err
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
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

	d.cfg.conf = conf

	return nil
}
