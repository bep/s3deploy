// Copyright © 2022 Bjørn Erik Pedersen <bjorn.erik.pedersen@gmail.com>.
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package lib

import (
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
)

func newAWSConfig(cfg *Config) (aws.Config, error) {
	return aws.Config{
		Region:      cfg.RegionName,
		Credentials: createCredentials(cfg),
	}, nil
}

func createCredentials(cfg *Config) aws.CredentialsProvider {

	if cfg.AccessKey != "" {
		return credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, os.Getenv("AWS_SESSION_TOKEN"))
	}

	// Use AWS default
	return nil
}
