// Copyright © 2018 Bjørn Erik Pedersen <bjorn.erik.pedersen@gmail.com>.
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package lib

import (
	"flag"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFlagsToConfig(t *testing.T) {
	assert := require.New(t)
	flags := flag.NewFlagSet("test", flag.PanicOnError)
	args := []string{
		"-bucket=mybucket",
		"-config=myconfig",
		"-force=true",
		"-key=mykey",
		"-secret=mysecret",
		"-max-delete=42",
		"-acl=public-read",
		"-path=mypath",
		"-quiet=true",
		"-region=myregion",
		"-source=mysource",
		"-distribution-id=mydistro",
		"-ignore=^ignored-prefix.*",
		"-try=true",
	}

	cfg, err := flagsToConfig(flags)
	assert.NoError(err)
	assert.NoError(flags.Parse(args))
	assert.Equal("mybucket", cfg.BucketName)
	assert.Equal("myconfig", cfg.ConfigFile)
	assert.Equal(true, cfg.Force)
	assert.Equal("mykey", cfg.AccessKey)
	assert.Equal("mysecret", cfg.SecretKey)
	assert.Equal(42, cfg.MaxDelete)
	assert.Equal("public-read", cfg.ACL)
	assert.Equal("mypath", cfg.BucketPath)
	assert.Equal(true, cfg.Silent)
	assert.Equal("mysource", cfg.SourcePath)
	assert.Equal(true, cfg.Try)
	assert.Equal("myregion", cfg.RegionName)
	assert.Equal(Strings{"mydistro"}, cfg.CDNDistributionIDs)
	assert.Equal("^ignored-prefix.*", cfg.Ignore)
}

func TestSetAclAndPublicAccessFlag(t *testing.T) {
	assert := require.New(t)
	flags := flag.NewFlagSet("test", flag.PanicOnError)
	args := []string{
		"-bucket=mybucket",
		"-acl=public-read",
		"-public-access=true",
	}

	cfg, err := flagsToConfig(flags)
	assert.NoError(err)
	assert.NoError(flags.Parse(args))

	check_err := cfg.check()
	assert.Error(check_err)
	assert.Contains(check_err.Error(), "you passed a value for the flags public-access and acl")
}

func TestIgnoreFlagError(t *testing.T) {
	assert := require.New(t)
	flags := flag.NewFlagSet("test", flag.PanicOnError)
	args := []string{
		"-bucket=mybucket",
		"-ignore=((INVALID_PATTERN",
	}

	cfg, err := flagsToConfig(flags)
	assert.NoError(err)
	assert.NoError(flags.Parse(args))

	check_err := cfg.check()
	assert.Error(check_err)
	assert.Contains(check_err.Error(), "cannot compile 'ignore' flag pattern")
}

func TestShouldIgnore(t *testing.T) {
	assert := require.New(t)

	flags_default := flag.NewFlagSet("test", flag.PanicOnError)
	flags_ignore := flag.NewFlagSet("test", flag.PanicOnError)

	args_default := []string{
		"-bucket=mybucket",
		"-path=my/path",
	}
	args_ignore := []string{
		"-bucket=mybucket",
		"-path=my/path",
		"-ignore=^ignored-prefix.*",
	}

	cfg_default, _ := flagsToConfig(flags_default)
	cfg_ignore, _ := flagsToConfig(flags_ignore)

	flags_default.Parse(args_default)
	flags_ignore.Parse(args_ignore)

	check_err_default := cfg_default.check()
	check_err_ignore := cfg_ignore.check()

	assert.NoError(check_err_default)
	assert.NoError(check_err_ignore)

	assert.False(cfg_default.shouldIgnoreLocal("any"))
	assert.False(cfg_default.shouldIgnoreLocal("ignored-prefix/file.txt"))

	assert.False(cfg_ignore.shouldIgnoreLocal("any"))
	assert.True(cfg_ignore.shouldIgnoreLocal("ignored-prefix/file.txt"))

	assert.False(cfg_default.shouldIgnoreRemote("my/path/any"))
	assert.False(cfg_default.shouldIgnoreRemote("my/path/ignored-prefix/file.txt"))

	assert.False(cfg_ignore.shouldIgnoreRemote("my/path/any"))
	assert.True(cfg_ignore.shouldIgnoreRemote("my/path/ignored-prefix/file.txt"))
}
