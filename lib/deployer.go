// Copyright © 2022 Bjørn Erik Pedersen <bjorn.erik.pedersen@gmail.com>.
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package lib

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/sync/errgroup"
	"golang.org/x/text/unicode/norm"
)

const up = `↑`

// Deployer deploys.
type Deployer struct {
	cfg   *Config
	stats *DeployStats

	g *errgroup.Group

	filesToUpload chan *osFile
	filesToDelete []string

	// Verbose output.
	outv io.Writer
	// Regular output.
	printer

	store remoteStore
}

// Deploy deploys to the remote based on the given config.
func Deploy(cfg *Config) (DeployStats, error) {
	if err := cfg.Init(); err != nil {
		return DeployStats{}, err
	}
	var outv, out io.Writer = io.Discard, os.Stdout
	if cfg.Silent {
		out = io.Discard
	} else {
		if cfg.Verbose {
			outv = os.Stdout
		}
		start := time.Now()
		defer func() {
			fmt.Printf("\nTotal in %.2f seconds\n", time.Since(start).Seconds())
		}()
	}

	var g *errgroup.Group
	ctx, cancel := context.WithCancel(context.Background())
	g, ctx = errgroup.WithContext(ctx)
	defer cancel()

	d := &Deployer{
		g:             g,
		outv:          outv,
		printer:       newPrinter(out),
		filesToUpload: make(chan *osFile),
		cfg:           cfg,
		stats:         &DeployStats{},
	}

	numberOfWorkers := cfg.NumberOfWorkers
	if numberOfWorkers <= 0 {
		numberOfWorkers = runtime.NumCPU()
	}

	baseStore := d.cfg.baseStore
	if baseStore == nil {
		var err error
		baseStore, err = newRemoteStore(ctx, d.cfg, d)
		if err != nil {
			return *d.stats, err
		}
	}
	if d.cfg.Try {
		baseStore = newNoUpdateStore(baseStore)
		d.Println("This is a trial run, with no remote updates.")
	}
	d.store = newStore(d.cfg, baseStore)

	for i := 0; i < numberOfWorkers; i++ {
		g.Go(func() error {
			return d.upload(ctx)
		})
	}

	err := d.plan(ctx)
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

	if err == nil {
		err = d.store.Finalize(context.Background())
	}

	return *d.stats, err
}

type printer interface {
	Println(a ...interface{}) (n int, err error)
	Printf(format string, a ...interface{}) (n int, err error)
}

type print struct {
	out io.Writer
}

func newPrinter(out io.Writer) printer {
	return print{out: out}
}

func (p print) Println(a ...interface{}) (n int, err error) {
	return fmt.Fprintln(p.out, a...)
}

func (p print) Printf(format string, a ...interface{}) (n int, err error) {
	return fmt.Fprintf(p.out, format, a...)
}

func (d *Deployer) printf(format string, a ...interface{}) {
	fmt.Fprintf(d.outv, format, a...)
}

func (d *Deployer) enqueueUpload(ctx context.Context, f *osFile) {
	d.Printf("%s (%s) %s ", f.keyPath, f.reason, up)
	select {
	case <-ctx.Done():
	case d.filesToUpload <- f:
	}
}

func (d *Deployer) skipFile(f *osFile) {
	d.printf("%s skipping …\n", f.relPath)
	atomic.AddUint64(&d.stats.Skipped, uint64(1))
}

func (d *Deployer) enqueueDelete(key string) {
	d.printf("%s not found in source, deleting.\n", key)
	d.filesToDelete = append(d.filesToDelete, key)
}

type uploadReason string

const (
	reasonNotFound uploadReason = "not found"
	reasonForce    uploadReason = "force"
	reasonSize     uploadReason = "size"
	reasonETag     uploadReason = "ETag"
)

// plan figures out which files need to be uploaded.
func (d *Deployer) plan(ctx context.Context) error {
	remoteFiles, err := d.store.FileMap(ctx)
	if err != nil {
		return err
	}
	d.printf("Found %d remote files\n", len(remoteFiles))

	// All local files at sourcePath
	localFiles := make(chan *osFile)
	d.g.Go(func() error {
		return d.walk(ctx, d.cfg.SourcePath, localFiles)
	})

	for f := range localFiles {
		// default: upload because local file not found on remote.
		up := true
		reason := reasonNotFound

		bucketPath := f.keyPath
		if d.cfg.BucketPath != "" {
			bucketPath = pathJoin(d.cfg.BucketPath, bucketPath)
		}

		if remoteFile, ok := remoteFiles[bucketPath]; ok {
			if d.cfg.Force {
				up = true
				reason = reasonForce
			} else {
				up, reason = f.shouldThisReplace(remoteFile)
			}
			// remove from map, whatever is leftover should be deleted:
			delete(remoteFiles, bucketPath)
		}

		f.reason = reason

		if up {
			d.enqueueUpload(ctx, f)
		} else {
			d.skipFile(f)
		}
	}
	close(d.filesToUpload)

	// any remote files not found locally should be removed:
	// except for ignored files
	for key := range remoteFiles {
		if d.cfg.shouldIgnoreRemote(key) {
			d.printf("%s ignored …\n", key)
			continue
		}
		d.enqueueDelete(key)
	}

	return nil
}

// walk a local directory
func (d *Deployer) walk(ctx context.Context, basePath string, files chan<- *osFile) error {
	err := filepath.Walk(basePath, func(fpath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		pathUnix := path.Clean(filepath.ToSlash(strings.TrimPrefix(fpath, basePath)))

		if info.IsDir() {
			if d.cfg.skipLocalDirs(pathUnix) {
				return filepath.SkipDir
			}
			return nil
		} else {
			if d.cfg.skipLocalFiles(pathUnix) {
				return nil
			}
		}

		if runtime.GOOS == "darwin" {
			// When a file system is HFS+, its filepath is in NFD form.
			fpath = norm.NFC.String(fpath)
		}

		abs, err := filepath.Abs(fpath)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(basePath, fpath)
		if err != nil {
			return err
		}

		if d.cfg.shouldIgnoreLocal(rel) {
			return nil
		}

		f, err := newOSFile(d.cfg, rel, abs, info)
		if err != nil {
			return err
		}

		if f.route != nil && f.route.Ignore {
			return nil
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
