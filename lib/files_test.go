// Copyright © 2022 Bjørn Erik Pedersen <bjorn.erik.pedersen@gmail.com>.
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package lib

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	qt "github.com/frankban/quicktest"
)

func TestOSFile(t *testing.T) {
	c := qt.New(t)

	of, err := openTestFile("main.css")
	c.Assert(err, qt.IsNil)

	c.Assert(of.Size(), qt.Equals, int64(3))
	c.Assert(of.ETag(), qt.Equals, `"902fbdd2b1df0c4f70b4a5d23525e932"`)
	c.Assert(of.Content(), qt.IsNotNil)
	b, err := ioutil.ReadAll(of.Content())
	c.Assert(err, qt.IsNil)
	c.Assert(string(b), qt.Equals, "ABC")
	c.Assert(of.ContentType(), qt.Equals, "text/css; charset=utf-8")
}

func TestShouldThisReplace(t *testing.T) {
	c := qt.New(t)

	of, err := openTestFile("main.css")
	c.Assert(err, qt.IsNil)

	correctETag := `"902fbdd2b1df0c4f70b4a5d23525e932"`

	for _, test := range []struct {
		testFile
		expect       bool
		expectReason string
	}{
		{testFile{"k1", int64(123), correctETag}, true, "size"},
		{testFile{"k2", int64(3), "FOO"}, true, "ETag"},
		{testFile{"k3", int64(3), correctETag}, false, ""},
	} {
		b, reason := of.shouldThisReplace(test.testFile)
		c.Assert(b, qt.Equals, test.expect)
		c.Assert(reason, qt.Equals, uploadReason(test.expectReason))
	}
}

func TestDetectContentTypeFromContent(t *testing.T) {
	c := qt.New(t)

	c.Assert(detectContentTypeFromContent([]byte("<html>foo</html>")), qt.Equals, "text/html; charset=utf-8")
	c.Assert(detectContentTypeFromContent([]byte("<html>"+strings.Repeat("abc", 300)+"</html>")), qt.Equals, "text/html; charset=utf-8")
}

type testFile struct {
	key  string
	size int64
	etag string
}

func (f testFile) Key() string {
	return f.key
}

func (f testFile) ETag() string {
	return f.etag
}

func (f testFile) Size() int64 {
	return f.size
}

func openTestFile(name string) (*osFile, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	relPath := filepath.Join("testdata", name)
	absPath := filepath.Join(wd, relPath)
	fi, err := os.Stat(absPath)
	if err != nil {
		return nil, err
	}

	return newOSFile(nil, "", relPath, absPath, fi)
}
