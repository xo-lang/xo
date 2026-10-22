# The runner machines' role: read seeds and the boot script, write CI's
# built binaries under dev/, and ask the register Lambda for a runner
# registration token. The GitHub App key is never readable from a machine.

data "aws_iam_policy_document" "ec2_assume" {
  statement {
    actions = ["sts:AssumeRole"]
    principals {
      type        = "Service"
      identifiers = ["ec2.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "runner" {
  name               = "xo-ci-runner"
  assume_role_policy = data.aws_iam_policy_document.ec2_assume.json
}

data "aws_iam_policy_document" "runner" {
  statement {
    actions   = ["s3:GetObject", "s3:ListBucket"]
    resources = ["arn:aws:s3:::${var.seed_bucket}", "arn:aws:s3:::${var.seed_bucket}/*"]
  }
  statement {
    actions   = ["s3:PutObject"]
    resources = ["arn:aws:s3:::${var.seed_bucket}/dev/*"]
  }
  statement {
    actions   = ["lambda:InvokeFunction"]
    resources = [aws_lambda_function.register.arn]
  }
}

resource "aws_iam_role_policy" "runner" {
  name   = "xo-ci-runner"
  role   = aws_iam_role.runner.id
  policy = data.aws_iam_policy_document.runner.json
}

# Session Manager on the runners (owner, 2026-10-22: live debugging of the
# Windows xo.exe crash; no inbound port, no keys)
resource "aws_iam_role_policy_attachment" "runner_ssm" {
  role       = aws_iam_role.runner.name
  policy_arn = "arn:aws:iam::aws:policy/AmazonSSMManagedInstanceCore"
}

resource "aws_iam_instance_profile" "runner" {
  name = "xo-ci-runner"
  role = aws_iam_role.runner.name
}

# The Lambdas: read the GitHub App's parameters; the autostart one also
# starts stopped runner machines.

data "aws_iam_policy_document" "lambda_assume" {
  statement {
    actions = ["sts:AssumeRole"]
    principals {
      type        = "Service"
      identifiers = ["lambda.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "lambda" {
  name               = "xo-ci-lambda"
  assume_role_policy = data.aws_iam_policy_document.lambda_assume.json
}

resource "aws_iam_role_policy_attachment" "lambda_logs" {
  role       = aws_iam_role.lambda.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole"
}

data "aws_caller_identity" "me" {}

data "aws_iam_policy_document" "lambda" {
  statement {
    actions   = ["ssm:GetParameter"]
    resources = ["arn:aws:ssm:${var.region}:${data.aws_caller_identity.me.account_id}:parameter/xo-ci/*"]
  }
  statement {
    actions   = ["ec2:DescribeInstances"]
    resources = ["*"]
  }
  statement {
    actions   = ["ec2:StartInstances"]
    resources = ["arn:aws:ec2:${var.region}:${data.aws_caller_identity.me.account_id}:instance/*"]
    condition {
      test     = "StringEquals"
      variable = "aws:ResourceTag/purpose"
      values   = ["xo-ci"]
    }
  }
}

resource "aws_iam_role_policy" "lambda" {
  name   = "xo-ci-lambda"
  role   = aws_iam_role.lambda.id
  policy = data.aws_iam_policy_document.lambda.json
}
