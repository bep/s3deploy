// Copyright © 2022 Bjørn Erik Pedersen <bjorn.erik.pedersen@gmail.com>.
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package lib

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudfront"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

var (
	_ remoteStore = (*s3Store)(nil)
	_ remoteCDN   = (*s3Store)(nil)
	_ file        = (*s3File)(nil)
)

type s3Store struct {
	bucket     string
	bucketPath string
	r          routes
	svc        *s3.Client
	acl        string
	cfc        *cloudFrontClient
}

type s3File struct {
	o types.Object
}

func (f *s3File) Key() string {
	return *f.o.Key
}

func (f *s3File) ETag() string {
	return *f.o.ETag
}

func (f *s3File) Size() int64 {
	return f.o.Size
}

func newRemoteStore(cfg Config, logger printer) (*s3Store, error) {
	var s *s3Store
	var cfc *cloudFrontClient

	awsConfig, err := newAWSConfig(cfg)
	if err != nil {
		return nil, err
	}

	cf := cloudfront.NewFromConfig(awsConfig)

	if len(cfg.CDNDistributionIDs) > 0 {
		cfc, err = newCloudFrontClient(cf, logger, cfg)
		if err != nil {
			return nil, err
		}
	}

	acl := "private"
	if cfg.ACL != "" {
		acl = cfg.ACL
	} else if cfg.PublicReadACL {
		acl = "public-read"
	}

	client := s3.NewFromConfig(awsConfig)

	s = &s3Store{svc: client, cfc: cfc, acl: acl, bucket: cfg.BucketName, r: cfg.conf.Routes, bucketPath: cfg.BucketPath}

	return s, nil
}

func (s *s3Store) FileMap(ctx context.Context, opts ...opOption) (map[string]file, error) {
	m := make(map[string]file)

	listObjectsV2Response, err := s.svc.ListObjectsV2(ctx,
		&s3.ListObjectsV2Input{
			Bucket: aws.String(s.bucket),
			Prefix: aws.String(s.bucketPath),
		})

	for {
		if err != nil {
			return nil, err
		}

		for _, o := range listObjectsV2Response.Contents {
			m[*o.Key] = &s3File{o: o}
		}

		if listObjectsV2Response.IsTruncated {
			listObjectsV2Response, err = s.svc.ListObjectsV2(ctx,
				&s3.ListObjectsV2Input{
					Bucket:            aws.String(s.bucket),
					Prefix:            aws.String(s.bucketPath),
					ContinuationToken: listObjectsV2Response.NextContinuationToken,
				},
			)
		} else {
			break
		}

	}

	return m, nil
}

func (s *s3Store) Put(ctx context.Context, f localFile, opts ...opOption) error {
	input := &s3.PutObjectInput{
		Bucket:        aws.String(s.bucket),
		Key:           aws.String(f.Key()),
		Body:          f.Content(),
		ACL:           types.ObjectCannedACL(s.acl),
		ContentType:   aws.String(f.ContentType()),
		ContentLength: f.Size(),
	}

	if err := s.applyMetadataToPutObjectInput(input, f); err != nil {
		return err
	}

	_, err := s.svc.PutObject(ctx, input)

	return err
}

func (s *s3Store) applyMetadataToPutObjectInput(input *s3.PutObjectInput, f localFile) error {
	m := f.Headers()
	if len(m) == 0 {
		return nil
	}

	if input.Metadata == nil {
		input.Metadata = make(map[string]string)
	}

	for k, v := range m {
		switch k {
		case "Cache-Control":
			input.CacheControl = aws.String(v)
		case "Content-Disposition":
			input.ContentDisposition = aws.String(v)
		case "Content-Encoding":
			input.ContentEncoding = aws.String(v)
		case "Content-Language":
			input.ContentLanguage = aws.String(v)
		case "Content-Type":
			// ContentType is already set.
		case "Expires":
			t, err := time.Parse(time.RFC1123, v)
			if err != nil {
				return fmt.Errorf("invalid Expires header: %s", err)
			}
			input.Expires = &t
		default:
			input.Metadata[k] = v
		}
	}

	return nil
}

func (s *s3Store) DeleteObjects(ctx context.Context, keys []string, opts ...opOption) error {
	ids := make([]types.ObjectIdentifier, len(keys))
	for i := 0; i < len(keys); i++ {
		ids[i] = types.ObjectIdentifier{Key: aws.String(keys[i])}
	}

	_, err := s.svc.DeleteObjects(ctx, &s3.DeleteObjectsInput{
		Bucket: aws.String(s.bucket),
		Delete: &types.Delete{
			Objects: ids,
		},
	})
	return err
}

func (s *s3Store) Finalize(ctx context.Context) error {
	return nil
}

func (s *s3Store) InvalidateCDNCache(ctx context.Context, paths ...string) error {
	if s.cfc == nil {
		return nil
	}
	return s.cfc.InvalidateCDNCache(ctx, paths...)
}
