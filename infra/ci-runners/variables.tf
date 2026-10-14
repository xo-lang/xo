variable "region" {
  type    = string
  default = "us-east-1"
}

variable "allowed_account" {
  description = "The only AWS account this configuration may touch."
  type        = string
  default     = "ACCOUNT_ID"
}

variable "github_owner" {
  type    = string
  default = "josephspurrier"
}

variable "github_repo" {
  type    = string
  default = "xo"
}

variable "seed_bucket" {
  description = "Private bucket of the seeds (decision 0216); also holds ci/boot/ and the dev/ binaries CI uploads."
  type        = string
  default     = "xo-seeds-ACCOUNT_ID"
}

variable "idle_minutes" {
  description = "A runner machine stops itself after this many minutes without a job."
  type        = number
  default     = 5
}

locals {
  repo_url = "https://github.com/${var.github_owner}/${var.github_repo}"
  # runners.json: name -> {type, arch, labels, import_id (optional: an
  # existing instance to adopt)}. Edit it with ./runners.sh.
  runners = jsondecode(file("${path.module}/runners.json"))
}
