// Copyright © 2022 Bjørn Erik Pedersen <bjorn.erik.pedersen@gmail.com>.
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package lib

import (
	"errors"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
)

func newAWSConfig(cfg Config) (aws.Config, error) {
	creds, err := createCredentials(cfg)
	if err != nil {
		return aws.Config{}, err
	}

	return aws.Config{
		Region:      cfg.RegionName,
		Credentials: creds,
	}, nil
}

func createCredentials(cfg Config) (aws.CredentialsProvider, error) {
	accessKey, secretKey := cfg.AccessKey, cfg.SecretKey

	if accessKey != "" && secretKey != "" {
		return credentials.NewStaticCredentialsProvider(accessKey, secretKey, os.Getenv("AWS_SESSION_TOKEN")), nil
	}

	if accessKey != "" || secretKey != "" {
		// provided one but not both
		return nil, errors.New("AWS key and secret are required")
	}

	if os.Getenv("AWS_ACCESS_KEY_ID") != "" {
		return credentials.NewStaticCredentialsProvider(os.Getenv("AWS_ACCESS_KEY_ID"), os.Getenv("AWS_SECRET_ACCESS_KEY"), os.Getenv("AWS_SESSION_TOKEN")), nil
	}

	// Use AWS default
	return nil, nil
}
