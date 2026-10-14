# Self-hosted GitHub Actions runners on the warm EC2 machines (decision
# 0225, infra/ci-runners/README.md).

terraform {
  required_version = ">= 1.10"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 6.0"
    }
    github = {
      source  = "integrations/github"
      version = "~> 6.0"
    }
    archive = {
      source  = "hashicorp/archive"
      version = "~> 2.0"
    }
    random = {
      source  = "hashicorp/random"
      version = "~> 3.0"
    }
  }
  backend "s3" {
    bucket       = "xo-tfstate-ACCOUNT_ID"
    key          = "ci-runners/terraform.tfstate"
    region       = "us-east-1"
    encrypt      = true
    use_lockfile = true
  }
}

provider "aws" {
  region              = var.region
  allowed_account_ids = [var.allowed_account]
  default_tags {
    tags = { project = "xo-ci-runners" }
  }
}

# The xo-lang-ci GitHub App (github-app/create-app.py stored its id, key,
# and installation in SSM).
data "aws_ssm_parameter" "app" {
  for_each        = toset(["app-id", "installation-id", "private-key"])
  name            = "/xo-ci/github-app/${each.key}"
  with_decryption = true
}

provider "github" {
  owner = var.github_owner
  app_auth {
    id              = data.aws_ssm_parameter.app["app-id"].value
    installation_id = data.aws_ssm_parameter.app["installation-id"].value
    pem_file        = data.aws_ssm_parameter.app["private-key"].value
  }
}
