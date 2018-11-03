// Copyright © 2018 Bjørn Erik Pedersen <bjorn.erik.pedersen@gmail.com>.
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package lib

import (
	"bytes"
	"compress/gzip"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"

	"github.com/dsnet/golib/memfile"
	"golang.org/x/text/unicode/norm"
)

var (
	_ file      = (*osFile)(nil)
	_ localFile = (*osFile)(nil)
	_ reasoner  = (*osFile)(nil)
)

type file interface {
	// Key represents the key on the target file store.
	Key() string
	ETag() string
	Size() int64
}

type reasoner interface {
	UploadReason() uploadReason
}

type localFile interface {
	file
	shouldThisReplace(other file) (bool, uploadReason)

	// Content returns the content to be stored remotely. If this file
	// configured to be gzipped, then that is what you get.
	Content() io.ReadSeeker

	Headers() map[string]string
}

type osStore struct{}

func newOSStore() *osStore {
	return &osStore{}
}

func (s *osStore) Walk(root string, walkFn WalkFunc) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		err = walkFn(path, info, err)
		if err == SkipDir {
			return filepath.SkipDir
		}
		return err
	})
}

func (s *osStore) IsHiddenDir(path string) bool {
	return strings.HasPrefix(path, ".")
}

func (s *osStore) IsIgnorableFilename(path string) bool {
	return path == ".DS_Store"
}

func (s *osStore) NormaliseName(path string) string {
	if runtime.GOOS == "darwin" {
		// When a file system is HFS+, its filepath is in NFD form.
		return norm.NFC.String(path)
	}
	return path
}

func (s *osStore) Abs(path string) (string, error) {
	return filepath.Abs(path)
}

func (s *osStore) Rel(basePath, path string) (string, error) {
	return filepath.Rel(basePath, path)
}

func (s *osStore) Ext(path string) string {
	return filepath.Ext(path)
}

func (s *osStore) ToSlash(path string) string {
	return filepath.ToSlash(path)
}

func (s *osStore) Open(path string) (io.ReadCloser, error) {
	return os.Open(path)
}

type osFile struct {
	relPath string

	// Filled when BucketPath is provided. Will store files in a sub-path
	// of the target file store.
	targetRoot string

	reason uploadReason

	absPath string
	size    int64

	etag     string
	etagInit sync.Once

	contentType string

	f *memfile.File

	route *route
}

func (f *osFile) Key() string {
	if f.targetRoot != "" {
		return path.Join(f.targetRoot, f.relPath)
	}
	return f.relPath
}

func (f *osFile) UploadReason() uploadReason {
	return f.reason
}

func (f *osFile) ETag() string {
	f.etagInit.Do(func() {
		var err error
		f.etag, err = calculateETag(f.Content())
		if err != nil {
			panic(err)
		}
	})
	return f.etag
}

func (f *osFile) Size() int64 {
	return f.size
}

func (f *osFile) Content() io.ReadSeeker {
	f.f.Seek(0, 0)
	return f.f
}

func (f *osFile) Headers() map[string]string {
	headers := map[string]string{
		"Content-Type": f.contentType,
	}

	if f.route != nil {
		if f.route.Gzip {
			headers["Content-Encoding"] = "gzip"
		}

		if f.route.Headers != nil {
			for k, v := range f.route.Headers {
				headers[k] = v
			}
		}
	}

	return headers
}

func (f *osFile) initContentType(local localStore, peek []byte) error {
	if f.route != nil {
		if contentType, found := f.route.Headers["Content-Type"]; found {
			f.contentType = contentType
			return nil
		}
	}

	contentType := mime.TypeByExtension(local.Ext(f.relPath))
	if contentType != "" {
		f.contentType = contentType
		return nil
	}

	// Have to look inside the file itself.
	if peek != nil {
		f.contentType = detectContentTypeFromContent(peek)
	} else {
		f.contentType = detectContentTypeFromContent(f.f.Bytes())
	}

	return nil
}

func detectContentTypeFromContent(b []byte) string {
	const magicSize = 512 // Size that DetectContentType expects
	var peek []byte

	if len(b) > magicSize {
		peek = b[:magicSize]
	} else {
		peek = b
	}

	return http.DetectContentType(peek)

}

func (f *osFile) shouldThisReplace(other file) (bool, uploadReason) {
	if f.Size() != other.Size() {
		return true, reasonSize
	}

	if f.ETag() != other.ETag() {
		return true, reasonETag
	}

	return false, ""
}

func newOSFile(local localStore, routes routes, targetRoot string, f *tmpFile) (*osFile, error) {
	relPath := local.ToSlash(f.relPath)

	file, err := local.Open(f.absPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open %q: %s", f.absPath, err)
	}
	defer file.Close()

	var (
		mFile *memfile.File
		size  = f.size
		peek  []byte
	)

	route := routes.get(relPath)

	if route != nil && route.Gzip {
		var b bytes.Buffer
		gz := gzip.NewWriter(&b)
		io.Copy(gz, file)
		gz.Close()
		mFile = memfile.New(b.Bytes())
		size = int64(b.Len())
		peek = make([]byte, 512)
		file.Read(peek)
	} else {
		b, err := ioutil.ReadAll(file)
		if err != nil {
			return nil, err
		}
		mFile = memfile.New(b)
	}

	of := &osFile{route: route, f: mFile, targetRoot: targetRoot, absPath: f.absPath, relPath: relPath, size: size}

	if err := of.initContentType(local, peek); err != nil {
		return nil, err
	}

	return of, nil
}

type tmpFile struct {
	relPath string
	absPath string
	size    int64
}

func newTmpFile(relPath, absPath string, size int64) *tmpFile {
	return &tmpFile{
		relPath: relPath,
		absPath: absPath,
		size:    size,
	}
}

type routes []*route

func (r routes) get(path string) *route {

	for _, route := range r {
		if route.routerRE.MatchString(path) {
			return route
		}
	}

	// no route found
	return nil

}

type orders []*regexp.Regexp

func (o orders) get(path string) int16 {
	for i := len(o) - 1; i >= 0; i-- {
		if o[i].MatchString(path) {
			return (int16)(i + 1)
		}
	}
	return 0
}

// read config from .s3deploy.yml if found.
type fileConfig struct {
	Routes routes   `yaml:"routes"`
	Order  []string `yaml:"order"`

	orderRE orders // compiled version of Order
}

// CompileResources compiles orders and routes
func (c *fileConfig) CompileResources() error {
	var err error
	for _, r := range c.Routes {
		r.routerRE, err = regexp.Compile(r.Route)

		if err != nil {
			return err
		}
	}

	c.orderRE = make([]*regexp.Regexp, len(c.Order), len(c.Order))
	for idx, o := range c.Order {
		c.orderRE[idx], err = regexp.Compile(o)

		if err != nil {
			return err
		}
	}
	return nil
}

type route struct {
	Route   string            `yaml:"route"`
	Headers map[string]string `yaml:"headers"`
	Gzip    bool              `yaml:"gzip"`

	routerRE *regexp.Regexp // compiled version of Route
}

func calculateETag(r io.Reader) (string, error) {

	h := md5.New()

	_, err := io.Copy(h, r)
	if err != nil {
		return "", err
	}
	return "\"" + hex.EncodeToString(h.Sum(nil)) + "\"", nil
}
