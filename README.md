# s3deploy

[![GoDoc](https://godoc.org/github.com/bep/s3deploy?status.svg)](https://godoc.org/github.com/bep/s3deploy)

A simple tool to deploy static websites to Amazon S3 with Gzip and custom headers support (e.g. "Cache-Control").

## Use

```bash
Usage of s3deploy:
  -bucket string
    	Destination bucket name on AWS
  -force
    	upload even if the etags match
  -h	help
  -key string
    	Access Key ID for AWS
  -region string
    	Name of region for AWS (default "us-east-1")
  -secret string
    	Secret Access Key for AWS
  -source string
    	path of files to upload (default ".")
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
      gzip: true
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

## Alternatives

* [go3up](https://github.com/alexaandru/go3up) by Alexandru Ungur
* [s3deploy](https://github.com/nathany/s3up) by Nathan Youngman (the starting-point of this project)
