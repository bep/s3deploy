// Copyright © 2018 Bjørn Erik Pedersen <bjorn.erik.pedersen@gmail.com>.
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package lib

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

var (
	_ remoteStore = (*testStore)(nil)
)

func TestDeploy(t *testing.T) {
	assert := require.New(t)
	store, m := newTestStore(0)
	source := testSourcePath()
	configFile := filepath.Join(source, ".s3deploy.yml")

	cfg := &Config{
		BucketName: "example.com",
		RegionName: "eu-west-1",
		ConfigFile: configFile,
		MaxDelete:  300,
		Silent:     true,
		SourcePath: source,
		baseStore:  store,
	}

	stats, err := Deploy(cfg)
	assert.NoError(err)
	assert.Equal("Deleted 1 of 1, uploaded 3, skipped 1 (80% changed)", stats.Summary())
	assertKeys(t, m, ".s3deploy.yml", "main.css", "index.html", "ab.txt")

	mainCss := m["main.css"]
	assert.IsType(&osFile{}, mainCss)
	headers := mainCss.(*osFile).Headers()
	assert.Equal("gzip", headers["Content-Encoding"])
	assert.Equal("text/css; charset=utf-8", headers["Content-Type"])
	assert.Equal("max-age=630720000, no-transform, public", headers["Cache-Control"])
}

func TestDeployForce(t *testing.T) {
	assert := require.New(t)
	store, _ := newTestStore(0)
	source := testSourcePath()

	cfg := &Config{
		BucketName: "example.com",
		RegionName: "eu-west-1",
		Force:      true,
		MaxDelete:  300,
		Silent:     true,
		SourcePath: source,
		baseStore:  store,
	}

	stats, err := Deploy(cfg)
	assert.NoError(err)
	assert.Equal("Deleted 1 of 1, uploaded 4, skipped 0 (100% changed)", stats.Summary())
}

func TestDeploySourceNotFound(t *testing.T) {
	assert := require.New(t)
	store, _ := newTestStore(0)
	wd, _ := os.Getwd()
	source := filepath.Join(wd, "thisdoesnotexist")

	cfg := &Config{
		BucketName: "example.com",
		RegionName: "eu-west-1",
		MaxDelete:  300,
		Silent:     true,
		SourcePath: source,
		baseStore:  store,
	}

	stats, err := Deploy(cfg)
	assert.Error(err)
	assert.Contains(err.Error(), "thisdoesnotexist")
	assert.Contains(stats.Summary(), "Deleted 0 of 0, uploaded 0, skipped 0")

}

func TestDeployInvalidSourcePath(t *testing.T) {
	assert := require.New(t)
	store, _ := newTestStore(0)
	root := "/"

	if runtime.GOOS == "windows" {
		root = `C:\`
	}

	cfg := &Config{
		BucketName: "example.com",
		RegionName: "eu-west-1",
		MaxDelete:  300,
		Silent:     true,
		SourcePath: root,
		baseStore:  store,
	}

	stats, err := Deploy(cfg)
	assert.Error(err)
	assert.Contains(err.Error(), "invalid source path")
	assert.Contains(stats.Summary(), "Deleted 0 of 0, uploaded 0, skipped 0")

}

func TestDeployNoBucket(t *testing.T) {
	assert := require.New(t)
	_, err := Deploy(&Config{Silent: true})
	assert.Error(err)
}

func TestDeployStoreFailures(t *testing.T) {
	for i := 1; i <= 3; i++ {
		assert := require.New(t)

		store, _ := newTestStore(i)
		source := testSourcePath()

		cfg := &Config{
			BucketName: "example.com",
			RegionName: "eu-west-1",
			MaxDelete:  300,
			Silent:     true,
			SourcePath: source,
			baseStore:  store,
		}

		message := fmt.Sprintf("Failure %d", i)

		stats, err := Deploy(cfg)
		assert.Error(err)

		if i == 3 {
			// Fail delete step
			assert.Contains(stats.Summary(), "Deleted 0 of 0, uploaded 3", message)
		} else {
			assert.Contains(stats.Summary(), "Deleted 0 of 0, uploaded 0", message)
		}
	}
}

func TestDeployMaxDelete(t *testing.T) {
	assert := require.New(t)

	m := make(map[string]file)

	for i := 0; i < 200; i++ {
		m[fmt.Sprintf("file%d.css", i)] = &testFile{}
	}

	store := newTestStoreFrom(m, 0)

	cfg := &Config{
		BucketName: "example.com",
		RegionName: "eu-west-1",
		Silent:     true,
		SourcePath: testSourcePath(),
		MaxDelete:  42,
		baseStore:  store,
	}

	stats, err := Deploy(cfg)
	assert.NoError(err)
	assert.Equal(158+4, len(m))
	assert.Equal("Deleted 42 of 200, uploaded 4, skipped 0 (100% changed)", stats.Summary())

}

func testSourcePath() string {
	wd, _ := os.Getwd()
	return filepath.Join(wd, "testdata")
}

func newTestStore(failAt int) (remoteStore, map[string]file) {
	m := map[string]file{
		"ab.txt":       &testFile{key: "ab.txt", etag: `"7b0ded95031647702b8bed17dce7698a"`, size: int64(3)},
		"main.css":     &testFile{key: "main.css", etag: `"changed"`, size: int64(27)},
		"deleteme.txt": &testFile{},
	}

	return newTestStoreFrom(m, failAt), m
}

func newTestStoreFrom(m map[string]file, failAt int) remoteStore {
	return &testStore{m: m, failAt: failAt}
}

type testStore struct {
	failAt int
	m      map[string]file
	remote map[string]file

	sync.Mutex
}

func assertKeys(t *testing.T, m map[string]file, keys ...string) {
	for _, k := range keys {
		if _, found := m[k]; !found {
			t.Fatal("key not found:", k)
		}
	}

	if len(keys) != len(m) {
		t.Log(m)
		t.Fatalf("map length mismatch: %d vs %d", len(keys), len(m))
	}
}

func (s *testStore) FileMap(opts ...opOption) (map[string]file, error) {
	s.Lock()
	defer s.Unlock()

	if s.failAt == 1 {
		return nil, errors.New("fail")
	}
	c := make(map[string]file)
	for k, v := range s.m {
		c[k] = v
	}
	return c, nil
}

func (s *testStore) Put(ctx context.Context, f localFile, opts ...opOption) error {
	s.Lock()
	defer s.Unlock()

	if s.failAt == 2 {
		return errors.New("fail")
	}
	s.m[f.Key()] = f
	return nil
}

func (s *testStore) DeleteObjects(ctx context.Context, keys []string, opts ...opOption) error {
	s.Lock()
	defer s.Unlock()

	if s.failAt == 3 {
		return errors.New("fail")
	}
	for _, k := range keys {
		delete(s.m, k)
	}
	return nil
}
