# One machine per runners.json entry. Entries with import_id adopt an
# existing warm machine (its disk and caches stay); the attributes that
# would force a new machine for those (AMI, subnet) are ignored.

data "aws_vpc" "default" {
  default = true
}

data "aws_subnet" "az" {
  for_each          = toset([for r in local.runners : lookup(r, "az", "us-east-1d")])
  vpc_id            = data.aws_vpc.default.id
  availability_zone = each.key
  default_for_az    = true
}

data "aws_ssm_parameter" "al2023" {
  for_each = toset(["arm64", "x86_64"])
  name     = "/aws/service/ami-amazon-linux-latest/al2023-ami-kernel-default-${each.key}"
}

data "aws_ssm_parameter" "windows" {
  name = "/aws/service/ami-windows-latest/Windows_Server-2025-English-Full-Base"
}

# The security group the warm machines already use (scripts/warm-run.sh);
# runners need no inbound access, SSH stays for debugging.
data "aws_security_group" "ssh" {
  name   = "xo-ci-ssh"
  vpc_id = data.aws_vpc.default.id
}

import {
  for_each = { for k, r in local.runners : k => r if lookup(r, "import_id", "") != "" }
  to       = aws_instance.runner[each.key]
  id       = each.value.import_id
}

resource "aws_instance" "runner" {
  for_each = local.runners

  ami                                  = lookup(each.value, "os", "linux") == "windows" ? data.aws_ssm_parameter.windows.value : data.aws_ssm_parameter.al2023[each.value.arch == "arm64" ? "arm64" : "x86_64"].value
  instance_type                        = each.value.type
  subnet_id                            = data.aws_subnet.az[lookup(each.value, "az", "us-east-1d")].id
  vpc_security_group_ids               = [data.aws_security_group.ssh.id]
  key_name                             = "xo-ci"
  iam_instance_profile                 = aws_iam_instance_profile.runner.name
  instance_initiated_shutdown_behavior = "stop"
  user_data                            = local.user_data[lookup(each.value, "os", "linux")]
  user_data_replace_on_change          = false

  metadata_options {
    http_tokens            = "required"
    instance_metadata_tags = "enabled"
  }

  root_block_device {
    volume_size = 120
    volume_type = "gp3"
  }

  tags = {
    Name        = "xo-ci-${each.key}"
    purpose     = "xo-ci"
    "ci-runner" = "xo-ci-${each.key}"
    "ci-labels" = join(",", each.value.labels)
    "ci-arch"   = each.value.arch
  }

  lifecycle {
    ignore_changes = [ami, subnet_id]
  }
}

# Runs on every boot: fetch the boot script from S3 and run it, so
# changing boot/linux.sh or boot/windows.ps1 never touches the machines'
# user data. Linux: cloud-init scripts-user "always". Windows: EC2Launch
# v2 runs <powershell> user data on every boot with <persist>true</persist>.
locals {
  user_data = {
    linux   = <<-EOT
      Content-Type: multipart/mixed; boundary="//"
      MIME-Version: 1.0

      --//
      Content-Type: text/cloud-config; charset="us-ascii"

      #cloud-config
      cloud_final_modules:
      - [scripts-user, always]

      --//
      Content-Type: text/x-shellscript; charset="us-ascii"

      #!/bin/bash
      export AWS_REGION=${var.region}
      aws s3 cp s3://${var.seed_bucket}/ci/boot/linux.sh /usr/local/sbin/xo-ci-boot.sh && bash /usr/local/sbin/xo-ci-boot.sh
      --//--
    EOT
    windows = <<-EOT
      <powershell>
      $env:AWS_REGION = '${var.region}'
      New-Item -ItemType Directory -Force C:\ProgramData\xo-ci | Out-Null
      Read-S3Object -BucketName ${var.seed_bucket} -Key ci/boot/windows.ps1 -File C:\ProgramData\xo-ci\boot.ps1 -Region ${var.region} | Out-Null
      & powershell -NoProfile -ExecutionPolicy Bypass -File C:\ProgramData\xo-ci\boot.ps1
      </powershell>
      <persist>true</persist>
    EOT
  }
}

resource "aws_s3_object" "boot_linux" {
  bucket = var.seed_bucket
  key    = "ci/boot/linux.sh"
  content = templatefile("${path.module}/boot/linux.sh", {
    repo_url     = local.repo_url
    region       = var.region
    idle_minutes = var.idle_minutes
    register_fn  = aws_lambda_function.register.function_name
  })
  content_type = "text/x-shellscript"
}

resource "aws_s3_object" "boot_windows" {
  bucket = var.seed_bucket
  key    = "ci/boot/windows.ps1"
  content = templatefile("${path.module}/boot/windows.ps1", {
    repo_url     = local.repo_url
    region       = var.region
    idle_minutes = var.idle_minutes
    register_fn  = aws_lambda_function.register.function_name
  })
  content_type = "text/plain"
}

output "runners" {
  value = { for k, i in aws_instance.runner : k => { id = i.id, type = i.instance_type, labels = local.runners[k].labels } }
}
