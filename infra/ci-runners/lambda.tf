# Two Lambdas sharing lambda/github.mjs: xo-ci-register (registration
# tokens for the machines) and xo-ci-autostart (the webhook and the
# one minute sweep that start stopped machines for queued jobs).

data "archive_file" "lambda" {
  type        = "zip"
  source_dir  = "${path.module}/lambda"
  output_path = "${path.module}/lambda/lambda.zip"
  excludes    = ["lambda.zip"]
}

resource "aws_lambda_function" "register" {
  function_name    = "xo-ci-register"
  role             = aws_iam_role.lambda.arn
  runtime          = "nodejs22.x"
  handler          = "register.handler"
  filename         = data.archive_file.lambda.output_path
  source_code_hash = data.archive_file.lambda.output_base64sha256
  timeout          = 30
  environment {
    variables = { GITHUB_REPO = "${var.github_owner}/${var.github_repo}" }
  }
}

resource "aws_lambda_function" "autostart" {
  function_name    = "xo-ci-autostart"
  role             = aws_iam_role.lambda.arn
  runtime          = "nodejs22.x"
  handler          = "autostart.handler"
  filename         = data.archive_file.lambda.output_path
  source_code_hash = data.archive_file.lambda.output_base64sha256
  timeout          = 60
  # No reserved concurrency: the account's limit is 10 and Lambda keeps 10
  # unreserved (gotchas/lambda-reserved-concurrency-account-minimum.md).
  # Two webhooks at once can start one machine too many; it stops itself
  # after var.idle_minutes.
  environment {
    variables = { GITHUB_REPO = "${var.github_owner}/${var.github_repo}" }
  }
}

resource "aws_lambda_function_url" "autostart" {
  function_name      = aws_lambda_function.autostart.function_name
  authorization_type = "NONE" # GitHub's HMAC signature is checked in the handler
}

resource "aws_cloudwatch_event_rule" "sweep" {
  name                = "xo-ci-autostart-sweep"
  schedule_expression = "rate(1 minute)"
}

resource "aws_cloudwatch_event_target" "sweep" {
  rule  = aws_cloudwatch_event_rule.sweep.name
  arn   = aws_lambda_function.autostart.arn
  input = jsonencode({ sweep = true })
}

resource "aws_lambda_permission" "sweep" {
  statement_id  = "sweep"
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.autostart.function_name
  principal     = "events.amazonaws.com"
  source_arn    = aws_cloudwatch_event_rule.sweep.arn
}

resource "aws_cloudwatch_log_group" "lambda" {
  for_each          = toset(["xo-ci-register", "xo-ci-autostart"])
  name              = "/aws/lambda/${each.key}"
  retention_in_days = 14
}
