// Copyright © 2022 Bjørn Erik Pedersen <bjorn.erik.pedersen@gmail.com>.
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package lib

import (
	"context"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
)

func newAWSConfig(ctx context.Context, cfg *Config) (aws.Config, error) {
	// Build options for LoadDefaultConfig
	var opts []func(*config.LoadOptions) error

	if cfg.RegionName != "" {
		opts = append(opts, config.WithRegion(cfg.RegionName))
	}

	if cfg.AccessKey != "" {
		// If static creds are provided, inject them while still loading other defaults.
		opts = append(opts, config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, os.Getenv("AWS_SESSION_TOKEN")),
		))
	}

	// Load the SDK default config (credentials chain, profiles, SSO, shared config, etc.)
	return config.LoadDefaultConfig(ctx, opts...)
}
