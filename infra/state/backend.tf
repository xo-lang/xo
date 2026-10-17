terraform {
  # bucket: from backend.hcl (git-ignored; terraform init -backend-config=backend.hcl, decision 0249)
  backend "s3" {
    key          = "state/terraform.tfstate"
    region       = "us-east-1"
    encrypt      = true
    use_lockfile = true
  }
}
