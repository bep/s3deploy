package lib

import (
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewRemoteStoreNoAclProvided(t *testing.T) {
	assert := require.New(t)

	cfg := Config{
		BucketName: "example.com",
		RegionName: "us-east-1",
		ACL:        "",
		Silent:     true,
	}

	s, err := newRemoteStore(cfg, newPrinter(ioutil.Discard))
	assert.NoError(err)

	assert.Equal(s.acl, "private")
}

func TestNewRemoteStoreAclProvided(t *testing.T) {
	assert := require.New(t)

	cfg := Config{
		BucketName: "example.com",
		RegionName: "us-east-1",
		ACL:        "public-read",
		Silent:     true,
	}

	s, err := newRemoteStore(cfg, newPrinter(ioutil.Discard))
	assert.NoError(err)

	assert.Equal(s.acl, "public-read")
}

func TestNewRemoteStoreOtherCannedAclProvided(t *testing.T) {
	assert := require.New(t)

	cfg := Config{
		BucketName: "example.com",
		RegionName: "us-east-1",
		ACL:        "bucket-owner-full-control",
		Silent:     true,
	}

	s, err := newRemoteStore(cfg, newPrinter(ioutil.Discard))
	assert.NoError(err)

	assert.Equal(s.acl, "bucket-owner-full-control")
}

func TestNewRemoteStoreDeprecatedPublicReadACLFlaglProvided(t *testing.T) {
	assert := require.New(t)

	cfg := Config{
		BucketName:    "example.com",
		RegionName:    "us-east-1",
		PublicReadACL: true,
		ACL:           "",
		Silent:        true,
	}

	s, err := newRemoteStore(cfg, newPrinter(ioutil.Discard))
	assert.NoError(err)

	assert.Equal(s.acl, "public-read")
}
