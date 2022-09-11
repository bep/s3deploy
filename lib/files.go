// Copyright © 2022 Bjørn Erik Pedersen <bjorn.erik.pedersen@gmail.com>.
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
	"sync"

	"github.com/dsnet/golib/memfile"
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

	ContentType() string

	Headers() map[string]string
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

func (f *osFile) ContentType() string {
	return f.contentType
}

func (f *osFile) Content() io.ReadSeeker {
	f.f.Seek(0, 0)
	return f.f
}

func (f *osFile) Headers() map[string]string {
	headers := map[string]string{}

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

func (f *osFile) initContentType(peek []byte) error {
	if f.route != nil {
		if contentType, found := f.route.Headers["Content-Type"]; found {
			f.contentType = contentType
			return nil
		}
	}

	contentType := mime.TypeByExtension(filepath.Ext(f.relPath))
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

func newOSFile(routes routes, targetRoot, relPath, absPath string, fi os.FileInfo) (*osFile, error) {
	relPath = filepath.ToSlash(relPath)

	file, err := os.Open(absPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open %q: %s", absPath, err)
	}
	defer file.Close()

	var (
		mFile *memfile.File
		size  = fi.Size()
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

	of := &osFile{route: route, f: mFile, targetRoot: targetRoot, absPath: absPath, relPath: relPath, size: size}

	if err := of.initContentType(peek); err != nil {
		return nil, err
	}

	return of, nil
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

// read config from .s3deploy.yml if found.
type fileConfig struct {
	Routes routes `yaml:"routes"`
}

type route struct {
	Route   string            `yaml:"route"`
	Headers map[string]string `yaml:"headers"`
	Gzip    bool              `yaml:"gzip"`
	Ignore  bool              `yaml:"ignore"`

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
