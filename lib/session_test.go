package lib

import (
	"io"
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
	store, err := newRemoteStore(cfg, newPrinter(io.Discard))
	c.Assert(err, qt.IsNil)
	c.Assert(store, qt.Not(qt.IsNil))

	c.Assert(*store.svc.Options().BaseEndpoint, qt.Equals, "http://localhost:9000")
}
