// Copyright © 2018 Bjørn Erik Pedersen <bjorn.erik.pedersen@gmail.com>.
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package lib

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/sync/errgroup"

	"gopkg.in/yaml.v2"
)

const up = `↑`

// Deployer deploys.
type Deployer struct {
	cfg   *Config
	stats *DeployStats

	g *errgroup.Group

	filesToDelete []string

	// Verbose output.
	outv io.Writer
	// Regular output.
	printer

	store remoteStore
	local localStore
}

type upload struct {
	*osFile
	reason string
}

// Deploy deploys to the remote based on the given config.
func Deploy(cfg *Config) (DeployStats, error) {
	if err := cfg.check(); err != nil {
		return DeployStats{}, err
	}

	var outv, out io.Writer = ioutil.Discard, os.Stdout
	if cfg.Silent {
		out = ioutil.Discard
	} else {
		if cfg.Verbose {
			outv = os.Stdout
		}
		start := time.Now()
		defer func() {
			fmt.Printf("\nTotal in %.2f seconds\n", time.Since(start).Seconds())
		}()
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var d = &Deployer{
		outv:    outv,
		printer: newPrinter(out),
		cfg:     cfg,
		stats:   &DeployStats{}}

	numberOfWorkers := cfg.NumberOfWorkers
	if numberOfWorkers <= 0 {
		numberOfWorkers = runtime.NumCPU()
	}

	// load additional config from file if it exists
	err := d.loadConfig()
	if err != nil {
		return *d.stats, fmt.Errorf("Failed to load config from %s: %s", cfg.ConfigFile, err)
	}

	baseStore := d.cfg.baseStore
	if baseStore == nil {
		baseStore, err = newRemoteStore(*d.cfg, d)
		if err != nil {
			return *d.stats, err
		}
	}
	if d.cfg.Try {
		baseStore = newNoUpdateStore(baseStore)
		d.Println("This is a trial run, with no remote updates.")
	}
	d.store = newStore(*d.cfg, baseStore)
	d.local = newOSStore()

	return d.deploy(ctx, numberOfWorkers)
}

func (d *Deployer) deploy(ctx context.Context, numberOfWorkers int) (DeployStats, error) {
	localFilesGroupped, err := d.groupLocalFiles(ctx, d.local, d.cfg.SourcePath)
	if err != nil {
		return *d.stats, err
	}

	remoteFiles, err := d.store.FileMap()
	if err != nil {
		return *d.stats, err
	}

	for idxg, localFiles := range localFilesGroupped {
		if len(localFiles) == 0 {
			d.Println("Ignoring group %d because it's empty", idxg)
		} else {
			d.Println("Processing group %d", idxg)

			filesToUpload := make(chan *osFile)

			wg, ctx := errgroup.WithContext(ctx)

			for i := 0; i < numberOfWorkers; i++ {
				wg.Go(func() error {
					return d.upload(ctx, filesToUpload)
				})
			}

			if err := d.plan(ctx, localFiles, remoteFiles, filesToUpload); err != nil {
				return *d.stats, err
			}

			if errwg := wg.Wait(); errwg != nil {
				// We want to exit on error or canceled
				return *d.stats, errwg
			}
		}
	}

	// any remote files not found locally should be removed:
	for key := range remoteFiles {
		if !strings.HasPrefix(key, d.cfg.BucketPath) {
			// Not part of this site: Keep!
			continue
		}
		d.enqueueDelete(key)
	}

	err = d.store.DeleteObjects(
		context.Background(),
		d.filesToDelete,
		withDeleteStats(d.stats),
		withMaxDelete(d.cfg.MaxDelete))

	if err == nil {
		err = d.store.Finalize()
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

func (d *Deployer) skipFile(f *osFile) {
	fmt.Fprintf(d.outv, "%s skipping …\n", f.relPath)
	atomic.AddUint64(&d.stats.Skipped, uint64(1))
}

func (d *Deployer) enqueueDelete(key string) {
	fmt.Fprintf(d.outv, "%s not found in source, deleting.\n", key)
	d.filesToDelete = append(d.filesToDelete, key)
}

type uploadReason string

const (
	reasonNotFound uploadReason = "not found"
	reasonForce    uploadReason = "force"
	reasonSize     uploadReason = "size"
	reasonETag     uploadReason = "ETag"
)

// plan figures out which files present on current group need to be uploaded.
func (d *Deployer) plan(ctx context.Context, localFiles []*tmpFile, remoteFiles map[string]file, filesToUpload chan<- *osFile) error {
	for _, f := range localFiles {
		// default: upload because local file not found on remote.
		up := true
		reason := reasonNotFound

		bucketPath := f.relPath
		if d.cfg.BucketPath != "" {
			bucketPath = path.Join(d.cfg.BucketPath, bucketPath)
		}

		osf, err := newOSFile(d.local, d.cfg.conf.Routes, d.cfg.BucketPath, f)
		if err != nil {
			return err
		}

		if remoteFile, ok := remoteFiles[bucketPath]; ok {
			if d.cfg.Force {
				up = true
				reason = reasonForce
			} else {
				up, reason = osf.shouldThisReplace(remoteFile)
			}
			// remove from map, whatever is leftover should be deleted:
			delete(remoteFiles, bucketPath)
		}

		osf.reason = reason

		if up {
			d.Printf("%s (%s) %s ", osf.relPath, osf.reason, up)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case filesToUpload <- osf:
			}
		} else {
			d.skipFile(osf)
		}
	}

	close(filesToUpload)

	return nil
}

// walk a local directory
func (d *Deployer) groupLocalFiles(ctx context.Context, local localStore, basePath string) ([][]*tmpFile, error) {
	filesToProcessByGroup := make([][]*tmpFile, len(d.cfg.conf.orderRE)+1)

	err := local.Walk(basePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			// skip hidden directories like .git
			if path != basePath && local.IsHiddenDir(info.Name()) {
				return SkipDir
			}

			return nil
		}

		if local.IsIgnorableFilename(info.Name()) {
			return nil
		}

		path = local.NormaliseName(path)

		abs, err := local.Abs(path)
		if err != nil {
			return err
		}
		rel, err := local.Rel(basePath, path)
		if err != nil {
			return err
		}

		f := newTmpFile(rel, abs, info.Size())
		group := d.cfg.conf.orderRE.get(f.relPath)
		filesToProcessByGroup[group] = append(filesToProcessByGroup[group], f)

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		return nil
	})

	return filesToProcessByGroup, err
}

func (d *Deployer) upload(ctx context.Context, filesToUpload <-chan *osFile) error {
	for {
		select {
		case f, ok := <-filesToUpload:
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

	if err := yaml.Unmarshal(data, &conf); err != nil {
		return err
	}

	if err := conf.CompileResources(); err != nil {
		return err
	}

	d.cfg.conf = conf

	return nil
}
