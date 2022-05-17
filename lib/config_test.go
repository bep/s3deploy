// Copyright © 2018 Bjørn Erik Pedersen <bjorn.erik.pedersen@gmail.com>.
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package lib

import (
	"flag"
	"testing"

	qt "github.com/frankban/quicktest"
)

func TestFlagsToConfig(t *testing.T) {
	c := qt.New(t)
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
	c.Assert(err, qt.IsNil)
	c.Assert(flags.Parse(args), qt.IsNil)
	c.Assert(cfg.BucketName, qt.Equals, "mybucket")
	c.Assert(cfg.ConfigFile, qt.Equals, "myconfig")
	c.Assert(cfg.Force, qt.Equals, true)
	c.Assert(cfg.AccessKey, qt.Equals, "mykey")
	c.Assert(cfg.SecretKey, qt.Equals, "mysecret")
	c.Assert(cfg.MaxDelete, qt.Equals, 42)
	c.Assert(cfg.ACL, qt.Equals, "public-read")
	c.Assert(cfg.BucketPath, qt.Equals, "mypath")
	c.Assert(cfg.Silent, qt.Equals, true)
	c.Assert(cfg.SourcePath, qt.Equals, "mysource")
	c.Assert(cfg.Try, qt.Equals, true)
	c.Assert(cfg.RegionName, qt.Equals, "myregion")
	c.Assert(cfg.CDNDistributionIDs, qt.DeepEquals, Strings{"mydistro"})
	c.Assert(cfg.Ignore, qt.Equals, "^ignored-prefix.*")
}

func TestSetAclAndPublicAccessFlag(t *testing.T) {
	c := qt.New(t)
	flags := flag.NewFlagSet("test", flag.PanicOnError)
	args := []string{
		"-bucket=mybucket",
		"-acl=public-read",
		"-public-access=true",
	}

	cfg, err := flagsToConfig(flags)
	c.Assert(err, qt.IsNil)
	c.Assert(flags.Parse(args), qt.IsNil)

	check_err := cfg.check()
	c.Assert(check_err, qt.IsNotNil)
	c.Assert(check_err.Error(), qt.Contains, "you passed a value for the flags public-access and acl")
}

func TestIgnoreFlagError(t *testing.T) {
	c := qt.New(t)
	flags := flag.NewFlagSet("test", flag.PanicOnError)
	args := []string{
		"-bucket=mybucket",
		"-ignore=((INVALID_PATTERN",
	}

	cfg, err := flagsToConfig(flags)
	c.Assert(err, qt.IsNil)
	c.Assert(flags.Parse(args), qt.IsNil)

	check_err := cfg.check()
	c.Assert(check_err, qt.IsNotNil)
	c.Assert(check_err.Error(), qt.Contains, "cannot compile 'ignore' flag pattern")
}

func TestShouldIgnore(t *testing.T) {
	c := qt.New(t)

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

	c.Assert(check_err_default, qt.IsNil)
	c.Assert(check_err_ignore, qt.IsNil)

	c.Assert(cfg_default.shouldIgnoreLocal("any"), qt.IsFalse)
	c.Assert(cfg_default.shouldIgnoreLocal("ignored-prefix/file.txt"), qt.IsFalse)

	c.Assert(cfg_ignore.shouldIgnoreLocal("any"), qt.IsFalse)
	c.Assert(cfg_ignore.shouldIgnoreLocal("ignored-prefix/file.txt"), qt.IsTrue)

	c.Assert(cfg_default.shouldIgnoreRemote("my/path/any"), qt.IsFalse)
	c.Assert(cfg_default.shouldIgnoreRemote("my/path/ignored-prefix/file.txt"), qt.IsFalse)

	c.Assert(cfg_ignore.shouldIgnoreRemote("my/path/any"), qt.IsFalse)
	c.Assert(cfg_ignore.shouldIgnoreRemote("my/path/ignored-prefix/file.txt"), qt.IsTrue)
}
