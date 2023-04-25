package lib

import (
	"testing"

	qt "github.com/frankban/quicktest"
)

func TestNewAWSConfigWithCustomEndpoint(t *testing.T) {
	c := qt.New(t)

	cfg := &Config{
		BucketName:  "example.com",
		RegionName:  "us-east-1",
		EndpointURL: "http://localhost:9000",
		Silent:      true,
	}

	awsCfg, err := newAWSConfig(cfg)
	c.Assert(err, qt.IsNil)

	endpoint, err := awsCfg.EndpointResolverWithOptions.ResolveEndpoint("s3", "us-east-1")
	c.Assert(err, qt.IsNil)

	c.Assert("http://localhost:9000", qt.Equals, endpoint.URL)
}
