# s3up

A simple tool to deploy my static websites (work in progress).

Example IAM policy:

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
         "Resource":"arn:aws:s3:::bucketname"
      },
      {
         "Effect":"Allow",
         "Action":[
            "s3:PutObject",
            "s3:DeleteObject"
         ],
         "Resource":"arn:aws:s3:::bucketname/*"
      }
   ]
}
```

\* replace bucketname with your own.

### Alternatives

* [go3up](https://github.com/alexaandru/go3up) by Alexandru Ungur

