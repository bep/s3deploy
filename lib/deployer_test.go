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
	"path"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	qt "github.com/frankban/quicktest"
)

var (
	_ remoteStore = (*testStore)(nil)
)

func TestDeploy(t *testing.T) {
	c := qt.New(t)
	store, m := newTestStore(0, "")
	source := testSourcePath()
	configFile := filepath.Join(source, ".s3deploy.yml")

	cfg := &Config{
		BucketName: "example.com",
		RegionName: "eu-west-1",
		ConfigFile: configFile,
		MaxDelete:  300,
		ACL:        "public-read",
		Silent:     true,
		SourcePath: source,
		baseStore:  store,
	}

	stats, err := Deploy(cfg)
	c.Assert(err, qt.IsNil)
	c.Assert(stats.Summary(), qt.Equals, "Deleted 1 of 1, uploaded 3, skipped 1 (80% changed)")
	assertKeys(t, m, ".s3deploy.yml", "main.css", "index.html", "ab.txt")

	mainCss := m["main.css"]
	headers := mainCss.(*osFile).Headers()
	c.Assert(headers["Content-Encoding"], qt.Equals, "gzip")
	c.Assert(headers["Content-Type"], qt.Equals, "text/css; charset=utf-8")
	c.Assert(headers["Cache-Control"], qt.Equals, "max-age=630720000, no-transform, public")
}

func TestDeployWithBucketPath(t *testing.T) {
	c := qt.New(t)
	root := "my/path"
	store, m := newTestStore(0, root)
	source := testSourcePath()
	configFile := filepath.Join(source, ".s3deploy.yml")

	cfg := &Config{
		BucketName: "example.com",
		RegionName: "eu-west-1",
		ConfigFile: configFile,
		BucketPath: root,
		MaxDelete:  300,
		Silent:     false,
		SourcePath: source,
		baseStore:  store,
	}

	stats, err := Deploy(cfg)
	c.Assert(err, qt.IsNil)
	c.Assert(stats.Summary(), qt.Equals, "Deleted 1 of 1, uploaded 3, skipped 1 (80% changed)")
	assertKeys(t, m, "my/path/.s3deploy.yml", "my/path/main.css", "my/path/index.html", "my/path/ab.txt")
	mainCss := m["my/path/main.css"]
	c.Assert(mainCss.(*osFile).Key(), qt.Equals, "my/path/main.css")
	headers := mainCss.(*osFile).Headers()
	c.Assert(headers["Content-Encoding"], qt.Equals, "gzip")

}

func TestDeployForce(t *testing.T) {
	c := qt.New(t)
	store, _ := newTestStore(0, "")
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
	c.Assert(err, qt.IsNil)
	c.Assert(stats.Summary(), qt.Equals, "Deleted 1 of 1, uploaded 4, skipped 0 (100% changed)")
}

func TestDeployWitIgnorePattern(t *testing.T) {
	c := qt.New(t)
	root := "my/path"
	re := `^(main\.css|deleteme\.txt)$`

	store, m := newTestStore(0, root)
	source := testSourcePath()
	configFile := filepath.Join(source, ".s3deploy.yml")

	cfg := &Config{
		BucketName: "example.com",
		RegionName: "eu-west-1",
		ConfigFile: configFile,
		BucketPath: root,
		MaxDelete:  300,
		Silent:     false,
		SourcePath: source,
		baseStore:  store,
		Ignore:     re,
	}

	prevCss := m["my/path/main.css"]
	prevTag := prevCss.ETag()

	stats, err := Deploy(cfg)
	c.Assert(err, qt.IsNil)
	c.Assert(stats.Summary(), qt.Equals, "Deleted 0 of 0, uploaded 2, skipped 1 (67% changed)")
	assertKeys(t, m,
		"my/path/.s3deploy.yml",
		"my/path/index.html",
		"my/path/ab.txt",
		"my/path/deleteme.txt", // ignored: stale
		"my/path/main.css",     // ignored: not updated
	)
	mainCss := m["my/path/main.css"]
	c.Assert(prevTag, qt.Equals, mainCss.ETag())

}

func TestDeploySourceNotFound(t *testing.T) {
	c := qt.New(t)
	store, _ := newTestStore(0, "")
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
	c.Assert(err, qt.IsNotNil)
	c.Assert(err.Error(), qt.Contains, "thisdoesnotexist")
	c.Assert(stats.Summary(), qt.Contains, "Deleted 0 of 0, uploaded 0, skipped 0")

}

func TestDeployInvalidSourcePath(t *testing.T) {
	c := qt.New(t)
	store, _ := newTestStore(0, "")
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
	c.Assert(err, qt.IsNotNil)
	c.Assert(err.Error(), qt.Contains, "invalid source path")
	c.Assert(stats.Summary(), qt.Contains, "Deleted 0 of 0, uploaded 0, skipped 0")

}

func TestDeployNoBucket(t *testing.T) {
	c := qt.New(t)
	_, err := Deploy(&Config{Silent: true})
	c.Assert(err, qt.IsNotNil)
}

func TestDeployStoreFailures(t *testing.T) {
	for i := 1; i <= 3; i++ {
		c := qt.New(t)

		store, _ := newTestStore(i, "")
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
		c.Assert(err, qt.IsNotNil)

		if i == 3 {
			// Fail delete step
			c.Assert(stats.Summary(), qt.Contains, "Deleted 0 of 0, uploaded 3", qt.Commentf(message))
		} else {
			c.Assert(stats.Summary(), qt.Contains, "Deleted 0 of 0, uploaded 0", qt.Commentf(message))
		}
	}
}

func TestDeployMaxDelete(t *testing.T) {
	c := qt.New(t)

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
	c.Assert(err, qt.IsNil)
	c.Assert(len(m), qt.Equals, 158+4)
	c.Assert(stats.Summary(), qt.Equals, "Deleted 42 of 200, uploaded 4, skipped 0 (100% changed)")

}

func testSourcePath() string {
	wd, _ := os.Getwd()
	return filepath.Join(wd, "testdata") + "/"
}

func newTestStore(failAt int, root string) (remoteStore, map[string]file) {
	m := map[string]file{
		path.Join(root, "ab.txt"):       &testFile{key: path.Join(root, "ab.txt"), etag: `"b86fc6b051f63d73de262d4c34e3a0a9"`, size: int64(2)},
		path.Join(root, "main.css"):     &testFile{key: path.Join(root, "main.css"), etag: `"changed"`, size: int64(27)},
		path.Join(root, "deleteme.txt"): &testFile{},
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

func (s *testStore) Finalize() error {
	return nil
}
