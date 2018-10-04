# s3deploy

[![GoDoc](https://godoc.org/github.com/bep/s3deploy?status.svg)](https://godoc.org/github.com/bep/s3deploy)
[![Build Status](https://travis-ci.org/bep/s3deploy.svg)](https://travis-ci.org/bep/s3deploy)
[![Build status](https://ci.appveyor.com/api/projects/status/rofnubomdtpv5ht4?svg=true)](https://ci.appveyor.com/project/bep/s3deploy)
[![Go Report Card](https://goreportcard.com/badge/github.com/bep/s3deploy)](https://goreportcard.com/report/github.com/bep/s3deploy)
[![codecov](https://codecov.io/gh/bep/s3deploy/branch/master/graph/badge.svg)](https://codecov.io/gh/bep/s3deploy)
[![Release](https://img.shields.io/github/release/bep/s3deploy.svg?style=flat-square)](https://github.com/bep/s3deploy/releases/latest)

A simple tool to deploy static websites to Amazon S3 and CloudFront with Gzip and custom headers support (e.g. "Cache-Control"). It uses ETag hashes to check if a file has changed, which makes it optimal in combination with static site generators like [Hugo](https://github.com/gohugoio/hugo).

## Install

Pre-built binaries can be found [here](https://github.com/bep/s3deploy/releases/latest).

**s3deploy** is a [Go application](https://golang.org/doc/install), so you can also get and build it yourself via `go get`:

```bash
 go get -u -v github.com/bep/s3deploy
 ```
 
 To install on MacOS using Homebrew:

 ```bash
 brew install bep/tap/s3deploy
 ```

Note that `s3deploy` is a perfect tool to use with a continuous integration tool such as [CircleCI](https://circleci.com/). See [this static site](https://github.com/bep/bego.io) for a simple example of automated depoloyment of a Hugo site to Amazon S3 via `s3deploy`.  The most relevant files are [circle.yml](https://github.com/bep/bego.io/blob/master/circle.yml) and [.s3deploy.yml](https://github.com/bep/bego.io/blob/master/.s3deploy.yml). For another example, see [this](https://mostlygeek.com/posts/hugo-circle-s3-hosting/) tutorial that uses s3deploy with CircleCI.

## Use

```bash
Usage of s3deploy:
  -V	print version and exit
  -bucket string
    	destination bucket name on AWS
  -config string
    	optional config file (default ".s3deploy.yml")
  -distribution-id string
    	optional CDN distribution ID for cache invalidation
  -force
    	upload even if the etags match
  -h	help
  -key string
    	access key ID for AWS
  -max-delete int
    	maximum number of files to delete per deploy (default 256)
  -path string
    	optional bucket sub path
  -quiet
    	enable silent mode
  -region string
    	name of AWS region
  -secret string
    	secret access key for AWS
  -source string
    	path of files to upload (default ".")
  -try
    	trial run, no remote updates
  -v	enable verbose logging
  -workers int
    	number of workers to upload files (default -1)
```

### Notes

- The `key` and `secret` command flags can also be set with environment variables `AWS_ACCESS_KEY_ID` and `AWS_SECRET_ACCESS_KEY`.
- The `region` flag is the AWS API name for the region where your bucket resides. See the table below or the [AWS Regions](https://docs.aws.amazon.com/general/latest/gr/rande.html#s3_region) documentation file for an up-to-date version.

Bucket region | API value | Bucket region | API value
------------- | --------- | ------------- | ----------
Canada (Central) | `ca-central-1` | Asia Pacific (Mumbai) | `ap-south-1`
US East (Ohio) | `us-east-2` | Asia Pacific (Seoul) | `ap-northeast-2`
US East (N. Virginia) | `us-east-1` | Asia Pacific (Singapore) | `ap-southeast-1`
US West (N. California) | `us-west-1` | Asia Pacific (Sydney) | `ap-southeast-2`
US West (Oregon) | `us-west-2` | Asia Pacific (Tokyo) | `ap-northeast-1`
EU (Frankfurt)  | `eu-central-1` | China (Beijing) | `cn-north-1`
EU (Ireland) | `eu-west-1` | China (Ningxia) | `cn-northwest-1`
EU (London) | `eu-west-2`
EU (Paris) | `eu-west-3`
South America (São Paulo) | `sa-east-1`

## Global AWS Configuration

See https://docs.aws.amazon.com/sdk-for-go/api/aws/session/#hdr-Sessions_from_Shared_Config

The `AWS SDK` will fall back to credentials from `~/.aws/credentials`.

If you set the `AWS_SDK_LOAD_CONFIG` enviroment variable, it will also load shared config from `~/.aws/config` where you can set the global `region` to use if not provided etc.

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
	
## CloudFront CDN Cache Invalidation

If you have configured CloudFront CDN in front of your S3 bucket, you can supply the `distribution-id` as a flag. This will make sure to invalidate the cache for the updated files after the deployment to S3. Note that the AWS user must have the needed access rights.

Note that CloudFront allows [1,000 paths per month at no charge](https://aws.amazon.com/blogs/aws/simplified-multiple-object-invalidation-for-amazon-cloudfront/), so S3deploy tries to be smart about the invalidation strategy; we try to reduce the number of paths to 8. If that isn't possible, we will fall back to a full invalidation, e.g. "/*".

### Example IAM Policy With CloudFront Config

```json
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Effect": "Allow",
            "Action": [
                "s3:ListBucket",
                "s3:GetBucketLocation"
            ],
            "Resource": "arn:aws:s3:::<bucketname>"
        },
        {
            "Effect": "Allow",
            "Action": [
                "s3:PutObject",
                "s3:DeleteObject",
                "s3:PutObjectAcl"
            ],
            "Resource": "arn:aws:s3:::<bucketname>/*"
        },
        {
            "Effect": "Allow",
            "Action": [
                "cloudfront:GetDistribution",
                "cloudfront:CreateInvalidation"
            ],
            "Resource": "*"
        }
    ]
}
```

## Background Information

If you're looking at `s3deploy` then you've probably already seen the [`aws s3 sync` command](http://docs.aws.amazon.com/cli/latest/reference/s3/sync.html) - this command has a sync-strategy that is not optimised for static sites, it compares the **timestamp** and **size** of your files to decide whether to upload the file.

Because static-site generators can recreate **every** file (even if identical) the timestamp is updated and thus `aws s3 sync` will needlessly upload every single file. `s3deploy` on the other hand checks the etag hash to check for actual changes, and uses that instead.

## Alternatives

* [go3up](https://github.com/alexaandru/go3up) by Alexandru Ungur
* [s3up](https://github.com/nathany/s3up) by Nathan Youngman (the starting-point of this project)
