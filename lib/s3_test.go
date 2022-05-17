package lib

import (
	"io/ioutil"
	"testing"

	qt "github.com/frankban/quicktest"
)

func TestNewRemoteStoreNoAclProvided(t *testing.T) {
	c := qt.New(t)

	cfg := Config{
		BucketName: "example.com",
		RegionName: "us-east-1",
		ACL:        "",
		Silent:     true,
	}

	s, err := newRemoteStore(cfg, newPrinter(ioutil.Discard))
	c.Assert(err, qt.IsNil)

	c.Assert("private", qt.Equals, s.acl)
}

func TestNewRemoteStoreAclProvided(t *testing.T) {
	c := qt.New(t)

	cfg := Config{
		BucketName: "example.com",
		RegionName: "us-east-1",
		ACL:        "public-read",
		Silent:     true,
	}

	s, err := newRemoteStore(cfg, newPrinter(ioutil.Discard))
	c.Assert(err, qt.IsNil)

	c.Assert("public-read", qt.Equals, s.acl)
}

func TestNewRemoteStoreOtherCannedAclProvided(t *testing.T) {
	c := qt.New(t)

	cfg := Config{
		BucketName: "example.com",
		RegionName: "us-east-1",
		ACL:        "bucket-owner-full-control",
		Silent:     true,
	}

	s, err := newRemoteStore(cfg, newPrinter(ioutil.Discard))
	c.Assert(err, qt.IsNil)

	c.Assert("bucket-owner-full-control", qt.Equals, s.acl)
}

func TestNewRemoteStoreDeprecatedPublicReadACLFlaglProvided(t *testing.T) {
	c := qt.New(t)

	cfg := Config{
		BucketName:    "example.com",
		RegionName:    "us-east-1",
		PublicReadACL: true,
		ACL:           "",
		Silent:        true,
	}

	s, err := newRemoteStore(cfg, newPrinter(ioutil.Discard))
	c.Assert(err, qt.IsNil)

	c.Assert("public-read", qt.Equals, s.acl)
}
