// Copyright © 2018 Bjørn Erik Pedersen <bjorn.erik.pedersen@gmail.com>.
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package lib

import (
	"context"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/s3"
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
	svc        *s3.S3
	acl        string
	cfc        *cloudFrontClient
}

type s3File struct {
	o *s3.Object
}

func (f *s3File) Key() string {
	return *f.o.Key
}

func (f *s3File) ETag() string {
	return *f.o.ETag
}

func (f *s3File) Size() int64 {
	return *f.o.Size
}

func newRemoteStore(cfg Config, logger printer) (*s3Store, error) {
	var s *s3Store
	var cfc *cloudFrontClient

	sess, err := newSession(cfg)
	if err != nil {
		return nil, err
	}

	if cfg.CDNDistributionID != "" {
		cfc, err = newCloudFrontClient(sess, logger, cfg)
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

	s = &s3Store{svc: s3.New(sess), cfc: cfc, acl: acl, bucket: cfg.BucketName, r: cfg.conf.Routes, bucketPath: cfg.BucketPath}

	return s, nil

}

func (s *s3Store) FileMap(opts ...opOption) (map[string]file, error) {
	m := make(map[string]file)

	err := s.svc.ListObjectsPages(&s3.ListObjectsInput{
		Bucket: aws.String(s.bucket),
		Prefix: aws.String(s.bucketPath),
	}, func(res *s3.ListObjectsOutput, last bool) (shouldContinue bool) {
		for _, o := range res.Contents {
			m[*o.Key] = &s3File{o: o}
		}
		return true
	})
	if err != nil {
		return nil, err
	}

	return m, nil
}

func (s *s3Store) Put(ctx context.Context, f localFile, opts ...opOption) error {

	headers := f.Headers()

	withHeaders := func(r *request.Request) {
		for k, v := range headers {
			r.HTTPRequest.Header.Add(k, v)
		}
	}

	_, err := s.svc.PutObjectWithContext(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(s.bucket),
		Key:           aws.String(f.Key()),
		Body:          f.Content(),
		ACL:           aws.String(s.acl),
		ContentLength: aws.Int64(f.Size()),
	}, withHeaders)

	return err
}

func (s *s3Store) DeleteObjects(ctx context.Context, keys []string, opts ...opOption) error {
	ids := make([]*s3.ObjectIdentifier, len(keys))
	for i := 0; i < len(keys); i++ {
		ids[i] = &s3.ObjectIdentifier{Key: aws.String(keys[i])}
	}

	_, err := s.svc.DeleteObjectsWithContext(ctx, &s3.DeleteObjectsInput{
		Bucket: aws.String(s.bucket),
		Delete: &s3.Delete{
			Objects: ids,
		},
	})
	return err
}

func (s *s3Store) Finalize() error {
	return nil
}

func (s *s3Store) InvalidateCDNCache(paths ...string) error {
	if s.cfc == nil {
		return nil
	}
	return s.cfc.InvalidateCDNCache(paths...)
}
