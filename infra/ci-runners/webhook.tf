# GitHub sends workflow_job events to the autostart Lambda.

resource "random_password" "webhook" {
  length  = 48
  special = false
}

resource "aws_ssm_parameter" "webhook_secret" {
  name  = "/xo-ci/webhook-secret"
  type  = "SecureString"
  value = random_password.webhook.result
}

resource "github_repository_webhook" "workflow_job" {
  repository = var.github_repo
  events     = ["workflow_job"]
  active     = true
  configuration {
    url          = aws_lambda_function_url.autostart.function_url
    content_type = "json"
    secret       = random_password.webhook.result
    insecure_ssl = false
  }
}
