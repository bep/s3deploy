// Copyright © 2022 Bjørn Erik Pedersen <bjorn.erik.pedersen@gmail.com>.
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package lib

import (
	"context"
	"fmt"
	"io"
	"path"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudfront"
	"github.com/aws/aws-sdk-go-v2/service/cloudfront/types"
	qt "github.com/frankban/quicktest"
)

func TestReduceInvalidationPaths(t *testing.T) {
	c := qt.New(t)

	var client *cloudFrontClient

	c.Assert(client.normalizeInvalidationPaths("root", 5, false, "/root/index.html"), qt.DeepEquals, []string{"/root/"})
	c.Assert(client.normalizeInvalidationPaths("", 5, false, "/index.html"), qt.DeepEquals, []string{"/"})
	c.Assert(client.normalizeInvalidationPaths("", 5, true, "/a", "/b"), qt.DeepEquals, []string{"/*"})
	c.Assert(client.normalizeInvalidationPaths("root", 5, true, "/a", "/b"), qt.DeepEquals, []string{"/root/*"})
	c.Assert(client.normalizeInvalidationPaths("root", 5, false, "/root/b/"), qt.DeepEquals, []string{"/root/b/"})

	rootPlusMany := append([]string{"/index.html", "/styles.css"}, createFiles("css", false, 20)...)
	normalized := client.normalizeInvalidationPaths("", 5, false, rootPlusMany...)
	c.Assert(len(normalized), qt.DeepEquals, 3)
	c.Assert(normalized, qt.DeepEquals, []string{"/", "/css/*", "/styles.css"})

	rootPlusManyInDifferentFolders := append([]string{"/index.html", "/styles.css"}, createFiles("css", true, 20)...)
	c.Assert(client.normalizeInvalidationPaths("", 5, false, rootPlusManyInDifferentFolders...), qt.DeepEquals, []string{"/*"})

	rootPlusManyInDifferentFoldersNested := append([]string{"/index.html", "/styles.css"}, createFiles("blog", false, 10)...)
	rootPlusManyInDifferentFoldersNested = append(rootPlusManyInDifferentFoldersNested, createFiles("blog/l1", false, 10)...)
	rootPlusManyInDifferentFoldersNested = append(rootPlusManyInDifferentFoldersNested, createFiles("blog/l1/l2/l3/l5", false, 10)...)
	rootPlusManyInDifferentFoldersNested = append(rootPlusManyInDifferentFoldersNested, createFiles("blog/l1/l3", false, 10)...)
	rootPlusManyInDifferentFoldersNested = append(rootPlusManyInDifferentFoldersNested, createFiles("about/l1", true, 10)...)
	rootPlusManyInDifferentFoldersNested = append(rootPlusManyInDifferentFoldersNested, createFiles("about/l1/l2/l3", false, 10)...)

	// avoid situations where many changes in some HTML template triggers update in /images and similar
	normalized = client.normalizeInvalidationPaths("", 5, false, rootPlusManyInDifferentFoldersNested...)
	c.Assert(len(normalized), qt.Equals, 4)
	c.Assert(normalized, qt.DeepEquals, []string{"/", "/about/*", "/blog/*", "/styles.css"})

	changes := []string{"/hugoscss/categories/index.html", "/hugoscss/index.html", "/hugoscss/tags/index.html", "/hugoscss/post/index.html", "/hugoscss/post/hello-scss/index.html", "/hugoscss/styles/main.min.36816b22057425f8a5f66b73918446b0cd793c0c6125406c285948f507599d1e.css"}
	normalized = client.normalizeInvalidationPaths("/hugoscss", 3, false, changes...)
	c.Assert(normalized, qt.DeepEquals, []string{"/hugoscss/*"})

	changes = []string{"/a/b1/a.css", "/a/b2/b.css"}
	normalized = client.normalizeInvalidationPaths("/", 3, false, changes...)
	c.Assert(normalized, qt.DeepEquals, []string{"/a/b1/a.css", "/a/b2/b.css"})

	normalized = client.normalizeInvalidationPaths("/", 1, false, changes...)
	c.Assert(normalized, qt.DeepEquals, []string{"/a/*"})

	// Force
	normalized = client.normalizeInvalidationPaths("", 5, true, rootPlusManyInDifferentFoldersNested...)
	c.Assert(normalized, qt.DeepEquals, []string{"/*"})
	normalized = client.normalizeInvalidationPaths("root", 5, true, rootPlusManyInDifferentFoldersNested...)
	c.Assert(normalized, qt.DeepEquals, []string{"/root/*"})
}

func TestDetermineRootAndSubPath(t *testing.T) {
	c := qt.New(t)

	var client *cloudFrontClient

	check := func(bucketPath, originPath, expectWebContextRoot, expectSubPath string) {
		t.Helper()
		s1, s2 := client.determineRootAndSubPath(bucketPath, originPath)
		c.Assert(s1, qt.Equals, expectWebContextRoot)
		c.Assert(s2, qt.Equals, expectSubPath)
	}

	check("temp/forsale", "temp", "/forsale", "temp")
	check("/temp/forsale/", "temp", "/forsale", "temp")
	check("root", "root", "/", "root")
	check("root", "/root", "/", "root")
}

func TestPathsToInvalidationBatch(t *testing.T) {
	c := qt.New(t)

	var client *cloudFrontClient

	batch := client.pathsToInvalidationBatch("myref", "/path1/", "/path2/")

	c.Assert(batch, qt.IsNotNil)
	c.Assert(*batch.CallerReference, qt.Equals, "myref")
	c.Assert(int(*batch.Paths.Quantity), qt.Equals, 2)
}

func TestNewCloudFrontClient(t *testing.T) {
	c := qt.New(t)
	client, err := newCloudFrontClient(
		&mockCloudfrontHandler{},
		newPrinter(io.Discard),
		&Config{
			CDNDistributionIDs: Strings{"12345"},
			Force:              true,
			BucketPath:         "/mypath",
		},
	)
	c.Assert(err, qt.IsNil)
	c.Assert(client, qt.IsNotNil)
	c.Assert(client.distributionIDs[0], qt.Equals, "12345")
	c.Assert(client.bucketPath, qt.Equals, "/mypath")
	c.Assert(client.force, qt.Equals, true)
}

func createFiles(root string, differentFolders bool, num int) []string {
	files := make([]string, num)

	for i := range num {
		nroot := root
		if differentFolders {
			nroot = fmt.Sprintf("%s-%d", root, i)
		}
		files[i] = path.Join(nroot, fmt.Sprintf("file%d.css", i+1))
	}

	return files
}

type mockCloudfrontHandler struct{}

func (c *mockCloudfrontHandler) GetDistribution(ctx context.Context, params *cloudfront.GetDistributionInput, optFns ...func(*cloudfront.Options)) (*cloudfront.GetDistributionOutput, error) {
	return &cloudfront.GetDistributionOutput{
		Distribution: &types.Distribution{
			DomainName: aws.String("example.com"),
		},
	}, nil
}

func (c *mockCloudfrontHandler) CreateInvalidation(ctx context.Context, params *cloudfront.CreateInvalidationInput, optFns ...func(*cloudfront.Options)) (*cloudfront.CreateInvalidationOutput, error) {
	return &cloudfront.CreateInvalidationOutput{}, nil
}
