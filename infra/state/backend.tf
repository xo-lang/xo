terraform {
  backend "s3" {
    bucket       = "xo-tfstate-ACCOUNT_ID"
    key          = "state/terraform.tfstate"
    region       = "us-east-1"
    encrypt      = true
    use_lockfile = true
  }
}
