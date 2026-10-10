data "aws_caller_identity" "current" {}

# Profile pictures. Private: browsers never read the bucket directly — the API
# serves /media/… — they only upload to it with a presigned POST (ADR 0004).
resource "aws_s3_bucket" "media" {
  # Bucket names are global across all of AWS; the account id makes ours unique.
  bucket = "relay-media-${data.aws_caller_identity.current.account_id}-${var.region}"
  # Lets `terraform destroy` delete a bucket that still has pictures in it.
  # Fine for a learning project; in real production you'd want the opposite.
  force_destroy = true
}

resource "aws_s3_bucket_public_access_block" "media" {
  bucket                  = aws_s3_bucket.media.id
  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

resource "aws_s3_bucket_ownership_controls" "media" {
  bucket = aws_s3_bucket.media.id
  rule {
    object_ownership = "BucketOwnerEnforced" # no ACLs at all; IAM decides everything
  }
}

resource "aws_s3_bucket_server_side_encryption_configuration" "media" {
  bucket = aws_s3_bucket.media.id
  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm = "AES256"
    }
  }
}

# The browser POSTs uploads straight to the bucket: a cross-origin request, so
# the bucket must allow our site's origin — and only it.
resource "aws_s3_bucket_cors_configuration" "media" {
  bucket = aws_s3_bucket.media.id
  cors_rule {
    allowed_origins = ["https://${local.domain}"]
    allowed_methods = ["POST"]
    allowed_headers = ["*"]
    max_age_seconds = 3600
  }
}
