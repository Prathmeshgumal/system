# The instance's identity. The API gets temporary credentials for this role from
# the instance metadata service — no access keys exist anywhere (decision N).

resource "aws_iam_role" "app" {
  name = "relay-app"
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { Service = "ec2.amazonaws.com" }
      Action    = "sts:AssumeRole"
    }]
  })
}

# Least privilege: pictures in this one bucket, nothing else in the account.
resource "aws_iam_role_policy" "media" {
  name = "relay-media"
  role = aws_iam_role.app.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Sid      = "Avatars"
        Effect   = "Allow"
        Action   = ["s3:GetObject", "s3:PutObject", "s3:DeleteObject"]
        Resource = "${aws_s3_bucket.media.arn}/avatars/*"
      },
      {
        # Without ListBucket, S3 answers a missing object with 403 instead of 404.
        Sid       = "ListAvatars"
        Effect    = "Allow"
        Action    = "s3:ListBucket"
        Resource  = aws_s3_bucket.media.arn
        Condition = { StringLike = { "s3:prefix" = ["avatars/*"] } }
      },
    ]
  })
}

resource "aws_iam_instance_profile" "app" {
  name = "relay-app"
  role = aws_iam_role.app.name
}
