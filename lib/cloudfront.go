// Copyright © 2022 Bjørn Erik Pedersen <bjorn.erik.pedersen@gmail.com>.
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package lib

import (
	"context"
	"errors"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudfront"
	"github.com/aws/aws-sdk-go-v2/service/cloudfront/types"
)

var _ remoteCDN = (*cloudFrontClient)(nil)

type cloudFrontClient struct {
	// The CloudFront distribution IDs
	distributionIDs Strings

	// Will invalidate the entire cache, e.g. "/*"
	force      bool
	bucketPath string

	logger printer
	cf     cloudfrontHandler
}

func newCloudFrontClient(
	handler cloudfrontHandler,
	logger printer,
	cfg *Config,
) (*cloudFrontClient, error) {
	if len(cfg.CDNDistributionIDs) == 0 {
		return nil, errors.New("must provide one or more distribution ID")
	}
	return &cloudFrontClient{
		distributionIDs: cfg.CDNDistributionIDs,
		force:           cfg.Force,
		bucketPath:      cfg.BucketPath,
		logger:          logger,
		cf:              handler,
	}, nil
}

type cloudfrontHandler interface {
	GetDistribution(ctx context.Context, params *cloudfront.GetDistributionInput, optFns ...func(*cloudfront.Options)) (*cloudfront.GetDistributionOutput, error)
	CreateInvalidation(ctx context.Context, params *cloudfront.CreateInvalidationInput, optFns ...func(*cloudfront.Options)) (*cloudfront.CreateInvalidationOutput, error)
}

func (c *cloudFrontClient) InvalidateCDNCache(ctx context.Context, paths ...string) error {
	if len(paths) == 0 {
		return nil
	}

	invalidateForID := func(id string) error {
		dcfg, err := c.cf.GetDistribution(ctx, &cloudfront.GetDistributionInput{
			Id: &id,
		})
		if err != nil {
			return err
		}

		originPath := *dcfg.Distribution.DistributionConfig.Origins.Items[0].OriginPath
		var root string
		if originPath != "" || c.bucketPath != "" {
			var subPath string
			root, subPath = c.determineRootAndSubPath(c.bucketPath, originPath)
			if subPath != "" {
				for i, p := range paths {
					paths[i] = strings.TrimPrefix(p, subPath)
				}
			}
		}

		// This will try to reduce the number of invaldation paths to maximum 8.
		// If that isn't possible it will fall back to a full invalidation, e.g. "/*".
		// CloudFront allows 1000 free invalidations per month. After that they
		// cost money, so we want to keep this down.
		paths = c.normalizeInvalidationPaths(root, 8, c.force, paths...)

		if len(paths) > 10 {
			c.logger.Printf("Create CloudFront invalidation request for %d paths", len(paths))
		} else {
			c.logger.Printf("Create CloudFront invalidation request for %v", paths)
		}

		in := &cloudfront.CreateInvalidationInput{
			DistributionId:    &id,
			InvalidationBatch: c.pathsToInvalidationBatch(time.Now().Format("20060102150405"), paths...),
		}

		_, err = c.cf.CreateInvalidation(
			ctx,
			in,
		)

		return err
	}

	for _, id := range c.distributionIDs {
		if err := invalidateForID(id); err != nil {
			return err
		}
	}

	return nil
}

func (*cloudFrontClient) pathsToInvalidationBatch(ref string, paths ...string) *types.InvalidationBatch {
	cfpaths := &types.Paths{}
	for _, p := range paths {
		cfpaths.Items = append(cfpaths.Items, pathEscapeRFC1738(p))
	}

	qty := int32(len(paths))
	cfpaths.Quantity = &qty

	return &types.InvalidationBatch{
		CallerReference: &ref,
		Paths:           cfpaths,
	}
}

// determineRootAndSubPath takes the bucketPath, as set as a flag,
// and the originPath, as set in the CDN config, and
// determines the web context root and the sub path below this context.
func (c *cloudFrontClient) determineRootAndSubPath(bucketPath, originPath string) (webContextRoot string, subPath string) {
	if bucketPath == "" && originPath == "" {
		panic("one of bucketPath or originPath must be set")
	}
	bucketPath = strings.Trim(bucketPath, "/")
	originPath = strings.Trim(originPath, "/")

	webContextRoot = strings.TrimPrefix(bucketPath, originPath)
	if webContextRoot == "" {
		webContextRoot = "/"
	}

	if originPath != bucketPath {
		// If the bucket path is a prefix of the origin, these resources
		// are served from a sub path, e.g. https://example.com/foo.
		subPath = strings.TrimPrefix(originPath, bucketPath)
	} else {
		// Served from the root.
		subPath = bucketPath
	}

	return
}

// For path rules, see https://docs.aws.amazon.com/AmazonCloudFront/latest/DeveloperGuide/Invalidation.html
func (c *cloudFrontClient) normalizeInvalidationPaths(
	root string,
	threshold int,
	force bool,
	paths ...string,
) []string {
	if !strings.HasPrefix(root, "/") {
		root = "/" + root
	}

	matchAll := path.Join(root, "*")
	clearAll := []string{matchAll}

	if force {
		return clearAll
	}

	var normalized []string
	var maxlevels int

	for _, p := range paths {
		p = pathClean(p)
		if !strings.HasPrefix(p, "/") {
			p = "/" + p
		}
		levels := strings.Count(p, "/")
		if levels > maxlevels {
			maxlevels = levels
		}

		if strings.HasSuffix(p, "index.html") {
			dir := path.Dir(p)
			if !strings.HasSuffix(dir, "/") {
				dir += "/"
			}
			normalized = append(normalized, dir)
		} else {
			normalized = append(normalized, p)
		}
	}

	normalized = uniqueStrings(normalized)
	sort.Strings(normalized)

	if len(normalized) > threshold {
		if len(normalized) > threshold {
			for k := maxlevels; k > 0; k-- {
				for i, p := range normalized {
					if strings.Count(p, "/") > k {
						parts := strings.Split(strings.TrimPrefix(path.Dir(p), "/"), "/")
						if len(parts) > 1 {
							parts = parts[:len(parts)-1]
						}
						normalized[i] = "/" + path.Join(parts...) + "/*"
					}
				}
				normalized = uniqueStrings(normalized)
				if len(normalized) <= threshold {
					break
				}
			}

			if len(normalized) > threshold {
				// Give up.
				return clearAll
			}
		}
	}

	for _, pattern := range normalized {
		if pattern == matchAll {
			return clearAll
		}
	}

	return normalized
}

func uniqueStrings(s []string) []string {
	var unique []string
	set := map[string]interface{}{}
	for _, val := range s {
		if _, ok := set[val]; !ok {
			unique = append(unique, val)
			set[val] = val
		}
	}
	return unique
}
