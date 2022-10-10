// Copyright © 2022 Bjørn Erik Pedersen <bjorn.erik.pedersen@gmail.com>.
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package lib

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
)

var (
	_ remoteStore = (*store)(nil)
	_ remoteCDN   = (*noUpdateStore)(nil)
)

type remoteStore interface {
	FileMap(ctx context.Context, opts ...opOption) (map[string]file, error)
	Put(ctx context.Context, f localFile, opts ...opOption) error
	DeleteObjects(ctx context.Context, keys []string, opts ...opOption) error
	Finalize(ctx context.Context) error
}

type remoteCDN interface {
	InvalidateCDNCache(ctx context.Context, paths ...string) error
}

type store struct {
	cfg      *Config
	delegate remoteStore

	changedKeys []string
	changedMu   sync.Mutex
}

func newStore(cfg *Config, s remoteStore) remoteStore {
	return &store{cfg: cfg, delegate: s}
}

func (s *store) trackChanged(keys ...string) {
	s.changedMu.Lock()
	defer s.changedMu.Unlock()
	s.changedKeys = append(s.changedKeys, keys...)
}

func (s *store) FileMap(ctx context.Context, opts ...opOption) (map[string]file, error) {
	return s.delegate.FileMap(ctx, opts...)
}

func (s *store) Finalize(ctx context.Context) error {
	if cdn, ok := s.delegate.(remoteCDN); ok {
		return cdn.InvalidateCDNCache(ctx, s.changedKeys...)
	}
	return nil
}

func (s *store) Put(ctx context.Context, f localFile, opts ...opOption) error {
	conf, err := optsToConfig(opts...)
	if err != nil {
		return err
	}

	err = s.delegate.Put(ctx, f, opts...)

	if err == nil {
		s.trackChanged(f.Key())
		conf.statsCollector(1, 0)
	}

	return err
}

func (s *store) DeleteObjects(ctx context.Context, keys []string, opts ...opOption) error {
	if len(keys) == 0 {
		return nil
	}

	conf, err := optsToConfig(opts...)
	if err != nil {
		return err
	}

	if conf.maxDelete <= 0 {
		// Nothing to do.
		return nil
	}

	chunkSize := 1000 // This is the maximum supported by the AWS SDK.
	if conf.maxDelete < chunkSize {
		chunkSize = conf.maxDelete
	}

	keyChunks := chunkStrings(keys, chunkSize)
	deleted := 0

	for i := 0; i < len(keyChunks); i++ {
		keyChunk := keyChunks[i]

		err := s.delegate.DeleteObjects(ctx, keyChunk, opts...)
		if err != nil {
			return err
		}

		s.trackChanged(keyChunk...)
		deleted += len(keyChunk)
		conf.statsCollector(deleted, 0)
		if deleted >= conf.maxDelete {
			conf.statsCollector(0, len(keys)-deleted)
			break
		}
	}

	return nil
}

type noUpdateStore struct {
	readOps remoteStore
}

func newNoUpdateStore(base remoteStore) remoteStore {
	return &noUpdateStore{readOps: base}
}

func (s *noUpdateStore) FileMap(ctx context.Context, opts ...opOption) (map[string]file, error) {
	if s.readOps != nil {
		return s.readOps.FileMap(ctx, opts...)
	}
	return make(map[string]file), nil
}

func (s *noUpdateStore) Put(ctx context.Context, f localFile, opts ...opOption) error {
	return nil
}

func (s *noUpdateStore) DeleteObjects(ctx context.Context, keys []string, opts ...opOption) error {
	return nil
}

func (s *noUpdateStore) Finalize(ctx context.Context) error {
	if s.readOps != nil {
		return s.readOps.Finalize(ctx)
	}
	return nil
}

func (s *noUpdateStore) InvalidateCDNCache(ctx context.Context, paths ...string) error {
	sort.Strings(paths)
	fmt.Println("\nInvalidate CDN:", paths)
	return nil
}

type opConfig struct {
	maxDelete      int
	statsCollector func(handled, skipped int)
}

type opOption func(c *opConfig) error

func withMaxDelete(count int) opOption {
	return func(c *opConfig) error {
		c.maxDelete = count
		return nil
	}
}

func withUploadStats(stats *DeployStats) opOption {
	return func(c *opConfig) error {
		c.statsCollector = func(handled, skipped int) {
			atomic.AddUint64(&stats.Uploaded, uint64(handled))
			atomic.AddUint64(&stats.Skipped, uint64(skipped))
		}
		return nil
	}
}

func withDeleteStats(stats *DeployStats) opOption {
	return func(c *opConfig) error {
		c.statsCollector = func(handled, skipped int) {
			atomic.AddUint64(&stats.Deleted, uint64(handled))
			atomic.AddUint64(&stats.Stale, uint64(skipped))
		}
		return nil
	}
}

func optsToConfig(opts ...opOption) (*opConfig, error) {
	c := &opConfig{}
	for _, opt := range opts {
		if err := opt(c); err != nil {
			return c, err
		}
	}

	if c.statsCollector == nil {
		c.statsCollector = func(handled, skipped int) {}
	}

	return c, nil
}

func chunkStrings(s []string, size int) [][]string {
	if len(s) == 0 {
		return nil
	}

	var chunks [][]string

	for i := 0; i < len(s); i += size {
		end := i + size

		if end > len(s) {
			end = len(s)
		}

		chunks = append(chunks, s[i:end])
	}

	return chunks
}
