package lib

import (
	"context"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
)

// Note: this version requires callers to provide a context.Context.
// It's the recommended approach because LoadDefaultConfig requires a context.
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
	sdkCfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return aws.Config{}, err
	}

	if cfg.EndpointURL != "" {
		resolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
			return aws.Endpoint{
				URL: cfg.EndpointURL,
			}, nil
		})
		sdkCfg.EndpointResolverWithOptions = resolver
	}

	return sdkCfg, nil
}
