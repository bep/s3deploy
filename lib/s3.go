// Copyright © 2018 Bjørn Erik Pedersen <bjorn.erik.pedersen@gmail.com>.
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package lib

import (
	"context"
	"errors"
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
)

var (
	_ remoteStore = (*s3Store)(nil)
	_ file        = (*s3File)(nil)
)

type s3Store struct {
	bucket string
	r      routes
	svc    *s3.S3
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

func newRemoteStore(cfg Config) (remoteStore, error) {
	var s *s3Store

	creds, err := s.createCredentials(cfg)
	if err != nil {
		return nil, err
	}

	region := new(string)

	if cfg.RegionName != "" {
		region = &cfg.RegionName
	}

	sess, err := session.NewSessionWithOptions(session.Options{
		Config: aws.Config{
			// The region may be set in global config. See SharedConfigState.
			Region: region,

			// The credentials object to use when signing requests.
			// Uses -key and -secret from command line if provided.
			// Defaults to a chain of credential providers to search for
			// credentials in environment variables, shared credential file,
			// and EC2 Instance Roles.
			Credentials: creds,
		},
		// This is the default in session.NewSession, but let us be explicit.
		// The end user can override this with AWS_SDK_LOAD_CONFIG=1.
		// See https://docs.aws.amazon.com/sdk-for-go/api/aws/session/#hdr-Sessions_from_Shared_Config
		SharedConfigState: session.SharedConfigStateFromEnv,
	})

	if err != nil {
		return nil, err
	}

	s = &s3Store{svc: s3.New(sess), bucket: cfg.BucketName, r: cfg.conf.Routes}

	return newStore(s), nil

}

func (s *s3Store) FileMap(opts ...opOption) (map[string]file, error) {
	m := make(map[string]file)

	err := s.svc.ListObjectsPages(&s3.ListObjectsInput{
		Bucket: aws.String(s.bucket),
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
		ACL:           aws.String("public-read"),
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

func (s *s3Store) createCredentials(cfg Config) (*credentials.Credentials, error) {
	accessKey, secretKey := cfg.AccessKey, cfg.SecretKey

	if accessKey != "" && secretKey != "" {
		return credentials.NewStaticCredentials(accessKey, secretKey, os.Getenv("AWS_SESSION_TOKEN")), nil
	}

	if accessKey != "" || secretKey != "" {
		// provided one but not both
		return nil, errors.New("AWS key and secret are required")
	}

	// Use AWS default
	return nil, nil
}
