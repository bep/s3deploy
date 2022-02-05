// Copyright © 2018 Bjørn Erik Pedersen <bjorn.erik.pedersen@gmail.com>.
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package lib

import (
	"fmt"
	"io/ioutil"
	"path"
	"testing"

	"github.com/aws/aws-sdk-go/awstesting/mock"
	"github.com/stretchr/testify/require"
)

func TestReduceInvalidationPaths(t *testing.T) {
	assert := require.New(t)

	var client *cloudFrontClient

	assert.Equal([]string{"/root/"}, client.normalizeInvalidationPaths("root", 5, false, "/root/index.html"))
	assert.Equal([]string{"/"}, client.normalizeInvalidationPaths("", 5, false, "/index.html"))
	assert.Equal([]string{"/*"}, client.normalizeInvalidationPaths("", 5, true, "/a", "/b"))
	assert.Equal([]string{"/root/*"}, client.normalizeInvalidationPaths("root", 5, true, "/a", "/b"))

	rootPlusMany := append([]string{"/index.html", "/styles.css"}, createFiles("css", false, 20)...)
	normalized := client.normalizeInvalidationPaths("", 5, false, rootPlusMany...)
	assert.Equal(3, len(normalized))
	assert.Equal([]string{"/", "/css/*", "/styles.css"}, normalized)

	rootPlusManyInDifferentFolders := append([]string{"/index.html", "/styles.css"}, createFiles("css", true, 20)...)
	assert.Equal([]string{"/*"}, client.normalizeInvalidationPaths("", 5, false, rootPlusManyInDifferentFolders...))

	rootPlusManyInDifferentFoldersNested := append([]string{"/index.html", "/styles.css"}, createFiles("blog", false, 10)...)
	rootPlusManyInDifferentFoldersNested = append(rootPlusManyInDifferentFoldersNested, createFiles("blog/l1", false, 10)...)
	rootPlusManyInDifferentFoldersNested = append(rootPlusManyInDifferentFoldersNested, createFiles("blog/l1/l2/l3/l5", false, 10)...)
	rootPlusManyInDifferentFoldersNested = append(rootPlusManyInDifferentFoldersNested, createFiles("blog/l1/l3", false, 10)...)
	rootPlusManyInDifferentFoldersNested = append(rootPlusManyInDifferentFoldersNested, createFiles("about/l1", true, 10)...)
	rootPlusManyInDifferentFoldersNested = append(rootPlusManyInDifferentFoldersNested, createFiles("about/l1/l2/l3", false, 10)...)

	// avoid situations where many changes in some HTML template triggers update in /images and similar
	normalized = client.normalizeInvalidationPaths("", 5, false, rootPlusManyInDifferentFoldersNested...)
	assert.Equal(4, len(normalized))
	assert.Equal([]string{"/", "/about/*", "/blog/*", "/styles.css"}, normalized)

	changes := []string{"/hugoscss/categories/index.html", "/hugoscss/index.html", "/hugoscss/tags/index.html", "/hugoscss/post/index.html", "/hugoscss/post/hello-scss/index.html", "/hugoscss/styles/main.min.36816b22057425f8a5f66b73918446b0cd793c0c6125406c285948f507599d1e.css"}
	normalized = client.normalizeInvalidationPaths("/hugoscss", 3, false, changes...)
	assert.Equal([]string{"/hugoscss/*"}, normalized)

	changes = []string{"/a/b1/a.css", "/a/b2/b.css"}
	normalized = client.normalizeInvalidationPaths("/", 3, false, changes...)
	assert.Equal([]string{"/a/b1/a.css", "/a/b2/b.css"}, normalized)

	normalized = client.normalizeInvalidationPaths("/", 1, false, changes...)
	assert.Equal([]string{"/a/*"}, normalized)

	// Force
	normalized = client.normalizeInvalidationPaths("", 5, true, rootPlusManyInDifferentFoldersNested...)
	assert.Equal([]string{"/*"}, normalized)
	normalized = client.normalizeInvalidationPaths("root", 5, true, rootPlusManyInDifferentFoldersNested...)
	assert.Equal([]string{"/root/*"}, normalized)
}

func TestDetermineRootAndSubPath(t *testing.T) {
	assert := require.New(t)

	var client *cloudFrontClient

	check := func(bucketPath, originPath, expectWebContextRoot, expectSubPath string) {
		t.Helper()
		s1, s2 := client.determineRootAndSubPath(bucketPath, originPath)
		assert.Equal(expectWebContextRoot, s1)
		assert.Equal(expectSubPath, s2)
	}

	check("temp/forsale", "temp", "/forsale", "temp")
	check("/temp/forsale/", "temp", "/forsale", "temp")
	check("root", "root", "/", "root")
	check("root", "/root", "/", "root")
}

func TestPathsToInvalidationBatch(t *testing.T) {
	assert := require.New(t)

	var client *cloudFrontClient

	batch := client.pathsToInvalidationBatch("myref", "/path1/", "/path2/")

	assert.NotNil(batch)
	assert.Equal("myref", *batch.CallerReference)
	assert.Equal(2, int(*batch.Paths.Quantity))
}

func TestNewCloudFrontClient(t *testing.T) {
	assert := require.New(t)
	s := mock.Session
	c, err := newCloudFrontClient(s, newPrinter(ioutil.Discard), Config{
		CDNDistributionIDs: Strings{"12345"},
		Force:              true,
		BucketPath:         "/mypath",
	})
	assert.NoError(err)
	assert.NotNil(c)
	assert.Equal("12345", c.distributionIDs[0])
	assert.Equal("/mypath", c.bucketPath)
	assert.Equal(true, c.force)
}

func createFiles(root string, differentFolders bool, num int) []string {
	files := make([]string, num)

	for i := 0; i < num; i++ {
		nroot := root
		if differentFolders {
			nroot = fmt.Sprintf("%s-%d", root, i)
		}
		files[i] = path.Join(nroot, fmt.Sprintf("file%d.css", i+1))
	}

	return files
}
