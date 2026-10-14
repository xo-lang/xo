# The S3 bucket that holds the Terraform state of infra/ci-runners
# (decision 0225). Created once with local state, which is then moved
# into the bucket itself (infra/state/README.md).

terraform {
  required_version = ">= 1.10"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
}

variable "region" {
  type    = string
  default = "us-east-1"
}

variable "allowed_account" {
  description = "The only AWS account this configuration may touch."
  type        = string
  default     = "ACCOUNT_ID"
}

provider "aws" {
  region              = var.region
  allowed_account_ids = [var.allowed_account]
  default_tags {
    tags = { project = "xo-ci-runners" }
  }
}

resource "aws_s3_bucket" "state" {
  bucket = "xo-tfstate-${var.allowed_account}"
  lifecycle {
    prevent_destroy = true
  }
}

resource "aws_s3_bucket_versioning" "state" {
  bucket = aws_s3_bucket.state.id
  versioning_configuration {
    status = "Enabled"
  }
}

resource "aws_s3_bucket_public_access_block" "state" {
  bucket                  = aws_s3_bucket.state.id
  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

resource "aws_s3_bucket_server_side_encryption_configuration" "state" {
  bucket = aws_s3_bucket.state.id
  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm = "AES256"
    }
  }
}

resource "aws_s3_bucket_lifecycle_configuration" "state" {
  bucket = aws_s3_bucket.state.id
  rule {
    id     = "old-versions"
    status = "Enabled"
    filter {}
    noncurrent_version_expiration {
      noncurrent_days = 90
    }
  }
}

output "bucket" {
  value = aws_s3_bucket.state.id
}
