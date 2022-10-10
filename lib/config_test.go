// Copyright © 2022 Bjørn Erik Pedersen <bjorn.erik.pedersen@gmail.com>.
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package lib

import (
	"os"
	"path/filepath"
	"testing"

	qt "github.com/frankban/quicktest"
)

func TestConfigFromArgs(t *testing.T) {
	c := qt.New(t)
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
		"-distribution-id=mydistro1",
		"-distribution-id=mydistro2",
		"-ignore=^ignored-prefix.*",
		"-try=true",
	}

	cfg, err := ConfigFromArgs(args)
	c.Assert(err, qt.IsNil)
	c.Assert(cfg.Init(), qt.IsNil)
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
	c.Assert(cfg.CDNDistributionIDs, qt.DeepEquals, Strings{"mydistro1", "mydistro2"})
	c.Assert(cfg.Ignore, qt.Equals, "^ignored-prefix.*")
}

func TestConfigFromEnvAndFile(t *testing.T) {
	c := qt.New(t)
	dir := t.TempDir()
	os.Setenv("S3DEPLOY_REGION", "myenvregion")
	os.Setenv("S3TEST_MYPATH", "mypath")
	os.Setenv("S3TEST_GZIP", "true")
	os.Setenv("S3TEST_CACHE_CONTROL", "max-age=1234")
	cfgFile := filepath.Join(dir, "config.yml")
	c.Assert(os.WriteFile(cfgFile, []byte(`
bucket: mybucket
region: myregion
path: ${S3TEST_MYPATH}

routes:
    - route: "^.+\\.(a)$"
      headers:
         Cache-Control: "${S3TEST_CACHE_CONTROL}"
      gzip: true
    - route: "^.+\\.(b)$"
      headers:
         Cache-Control: "max-age=630720000, no-transform, public"
      gzip: false
    - route: "^.+\\.(c)$"
      gzip: "${S3TEST_GZIP@U}"
`), 0644), qt.IsNil)

	args := []string{
		"-config=" + cfgFile,
	}

	cfg, err := ConfigFromArgs(args)
	c.Assert(err, qt.IsNil)
	c.Assert(cfg.Init(), qt.IsNil)
	c.Assert(cfg.BucketName, qt.Equals, "mybucket")
	c.Assert(cfg.BucketPath, qt.Equals, "mypath")
	c.Assert(cfg.RegionName, qt.Equals, "myenvregion")
	routes := cfg.fileConf.Routes
	c.Assert(routes, qt.HasLen, 3)
	c.Assert(routes[0].Route, qt.Equals, "^.+\\.(a)$")
	c.Assert(routes[0].Headers["Cache-Control"], qt.Equals, "max-age=1234")
	c.Assert(routes[0].Gzip, qt.IsTrue)
	c.Assert(routes[2].Gzip, qt.IsTrue)

}

func TestConfigFromFileErrors(t *testing.T) {
	c := qt.New(t)
	dir := t.TempDir()
	cfgFileInvalidYaml := filepath.Join(dir, "config_invalid_yaml.yml")
	c.Assert(os.WriteFile(cfgFileInvalidYaml, []byte(`
bucket=foo
`), 0644), qt.IsNil)

	args := []string{
		"-config=" + cfgFileInvalidYaml,
	}

	_, err := ConfigFromArgs(args)
	c.Assert(err, qt.IsNotNil)

	cfgFileInvalidRoute := filepath.Join(dir, "config_invalid_route.yml")
	c.Assert(os.WriteFile(cfgFileInvalidRoute, []byte(`
bucket: foo
routes:
   - route: "*" # invalid regexp.
`), 0644), qt.IsNil)

	args = []string{
		"-config=" + cfgFileInvalidRoute,
	}

	cfg, err := ConfigFromArgs(args)
	c.Assert(err, qt.IsNil)
	err = cfg.Init()
	c.Assert(err, qt.IsNotNil)

}

func TestSetAclAndPublicAccessFlag(t *testing.T) {
	c := qt.New(t)
	args := []string{
		"-bucket=mybucket",
		"-acl=public-read",
		"-public-access=true",
	}

	cfg, err := ConfigFromArgs(args)
	c.Assert(err, qt.IsNil)

	err = cfg.Init()
	c.Assert(err, qt.IsNotNil)
	c.Assert(err.Error(), qt.Contains, "you passed a value for the flags public-access and acl")
}

func TestIgnoreFlagError(t *testing.T) {
	c := qt.New(t)
	args := []string{
		"-bucket=mybucket",
		"-ignore=((INVALID_PATTERN",
	}

	cfg, err := ConfigFromArgs(args)
	c.Assert(err, qt.IsNil)

	err = cfg.Init()
	c.Assert(err, qt.IsNotNil)
	c.Assert(err.Error(), qt.Contains, "cannot compile 'ignore' flag pattern")
}

func TestShouldIgnore(t *testing.T) {
	c := qt.New(t)

	argsDefault := []string{
		"-bucket=mybucket",
		"-path=my/path",
	}
	argsIgnore := []string{
		"-bucket=mybucket",
		"-path=my/path",
		"-ignore=^ignored-prefix.*",
	}

	cfgDefault, err := ConfigFromArgs(argsDefault)
	c.Assert(err, qt.IsNil)
	cfgIgnore, err := ConfigFromArgs(argsIgnore)
	c.Assert(err, qt.IsNil)

	c.Assert(cfgDefault.Init(), qt.IsNil)
	c.Assert(cfgIgnore.Init(), qt.IsNil)

	c.Assert(cfgDefault.shouldIgnoreLocal("any"), qt.IsFalse)
	c.Assert(cfgDefault.shouldIgnoreLocal("ignored-prefix/file.txt"), qt.IsFalse)

	c.Assert(cfgIgnore.shouldIgnoreLocal("any"), qt.IsFalse)
	c.Assert(cfgIgnore.shouldIgnoreLocal("ignored-prefix/file.txt"), qt.IsTrue)

	c.Assert(cfgDefault.shouldIgnoreRemote("my/path/any"), qt.IsFalse)
	c.Assert(cfgDefault.shouldIgnoreRemote("my/path/ignored-prefix/file.txt"), qt.IsFalse)

	c.Assert(cfgIgnore.shouldIgnoreRemote("my/path/any"), qt.IsFalse)
	c.Assert(cfgIgnore.shouldIgnoreRemote("my/path/ignored-prefix/file.txt"), qt.IsTrue)
}
