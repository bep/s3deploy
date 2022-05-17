// Copyright © 2018 Bjørn Erik Pedersen <bjorn.erik.pedersen@gmail.com>.
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package lib

import (
	"errors"
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
)

func newSession(cfg Config) (*session.Session, error) {
	creds, err := createCredentials(cfg)
	if err != nil {
		return nil, err
	}

	region := new(string)

	if cfg.RegionName != "" {
		region = &cfg.RegionName
	}

	return session.NewSessionWithOptions(session.Options{
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
}

func createCredentials(cfg Config) (*credentials.Credentials, error) {
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
