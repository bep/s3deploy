# s3deploy

[![GoDoc](https://godoc.org/github.com/bep/s3deploy?status.svg)](https://godoc.org/github.com/bep/s3deploy)
[![CircleCI](https://circleci.com/gh/bep/s3deploy.svg?style=svg)](https://circleci.com/gh/bep/s3deploy)
[![Release](https://img.shields.io/github/release/bep/s3deploy.svg?style=flat-square)](https://github.com/bep/s3deploy/releases/latest)

A simple tool to deploy static websites to Amazon S3 with Gzip and custom headers support (e.g. "Cache-Control").

## Install

Pre-built binaries can be found [here](https://github.com/bep/s3deploy/releases/latest).

**s3deploy** is a [Go application](https://golang.org/doc/install), so you can also get and build it yourself via `go get`:

```bash
 go get -v github.com/bep/s3deploy
 ```
 
Note that `s3deploy` is a perfect tool to use with a continuous integration tool such as [CircleCI](https://circleci.com/). See [this static site](https://github.com/bep/bego.io) for a simple example of automated depoloyment of a Hugo site to Amazon S3 via `s3deploy`.  The most relevant files are [circle.yml](https://github.com/bep/bego.io/blob/master/circle.yml) and [.s3deploy.yml](https://github.com/bep/bego.io/blob/master/.s3deploy.yml).

## Use

```bash
Usage of s3deploy:
  -V	print version and exit
  -bucket string
    	Destination bucket name on AWS
  -config string
    	optional config file (default ".s3deploy.yml")
  -force
    	upload even if the etags match
  -h	help
  -key string
    	Access Key ID for AWS
  -path string
    	Optional bucket sub path
  -region string
    	Name of region for AWS (default "us-east-1")
  -secret string
    	Secret Access Key for AWS
  -source string
    	path of files to upload (default ".")
  -v	enable verbose logging
  -workers int
    	number of workers to upload files (default -1)
```

**Note:** `key` and `secret` can also be set in environment variables `AWS_ACCESS_KEY_ID` and `AWS_SECRET_ACCESS_KEY`.


## Advanced Configuration

Add a `.s3deploy.yml` configuration file in the root of your site. Example configuration:

```yaml
routes:
    - route: "^.+\\.(js|css|svg|ttf)$"
      #  cache static assets for 20 years
      headers:
         Cache-Control: "max-age=630720000, no-transform, public"
      gzip: true
    - route: "^.+\\.(png|jpg)$"
      headers:
         Cache-Control: "max-age=630720000, no-transform, public"
      gzip: false
    - route: "^.+\\.(html|xml|json)$"
      gzip: true   
``` 


## Example IAM Policy

```json
{
   "Version": "2012-10-17",
   "Statement":[
      {
         "Effect":"Allow",
         "Action":[
            "s3:ListBucket",
            "s3:GetBucketLocation"
         ],
         "Resource":"arn:aws:s3:::<bucketname>"
      },
      {
         "Effect":"Allow",
         "Action":[
            "s3:PutObject",
            "s3:PutObjectAcl",
            "s3:DeleteObject"
         ],
         "Resource":"arn:aws:s3:::<bucketname>/*"
      }
   ]
}
```

Replace <bucketname> with your own.

## Background Information

If you're looking at `s3deploy` then you've probably already seen the [`aws s3 sync` command](http://docs.aws.amazon.com/cli/latest/reference/s3/sync.html) - this command has a sync-strategy that is not optimised for static sites, it compares the **timestamp** and **size** of your files to decide whether to upload the file.

Because static-site generators can recreate **every** file (even if identical) the timestamp is updated and thus `aws s3 sync` will needlessly upload every single file. `s3deploy` on the other hand checks the etag hash to check for actual changes, and uses that instead.

## Alternatives

* [go3up](https://github.com/alexaandru/go3up) by Alexandru Ungur
* [s3up](https://github.com/nathany/s3up) by Nathan Youngman (the starting-point of this project)
