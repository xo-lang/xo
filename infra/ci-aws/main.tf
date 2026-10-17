# Ephemeral Linux test machines for the Xo CI matrix (infra/ci-aws/README.md).
# Creates only: one key pair, one security group, and one instance per
# entry in var.instances. Everything else (default VPC, subnets, AMIs) is
# read through data sources.

terraform {
  required_version = ">= 1.5"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
    tls = {
      source  = "hashicorp/tls"
      version = "~> 4.0"
    }
    local = {
      source  = "hashicorp/local"
      version = "~> 2.0"
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
}

variable "ssh_cidr" {
  description = "The only source allowed to reach port 22."
  type        = string
}

variable "instances" {
  description = "Name suffix -> instance type and AMI architecture."
  type = map(object({
    type = string
    arch = string # arm64 or amd64 (Canonical's AMI name spelling)
  }))
  default = {
    arm64 = { type = "c7g.4xlarge", arch = "arm64" }
    amd64 = { type = "c7i.4xlarge", arch = "amd64" }
  }
}

provider "aws" {
  region              = var.region
  allowed_account_ids = [var.allowed_account]
  default_tags {
    tags = { project = "xo-ci" }
  }
}

data "aws_vpc" "default" {
  default = true
}

# Availability zones that offer every requested instance type.
data "aws_ec2_instance_type_offerings" "zones" {
  for_each      = var.instances
  location_type = "availability-zone"
  filter {
    name   = "instance-type"
    values = [each.value.type]
  }
}

locals {
  zone_sets = [for o in data.aws_ec2_instance_type_offerings.zones : toset(o.locations)]
  zones     = sort(tolist(setintersection(local.zone_sets...)))
}

data "aws_subnets" "default" {
  filter {
    name   = "vpc-id"
    values = [data.aws_vpc.default.id]
  }
  filter {
    name   = "default-for-az"
    values = ["true"]
  }
  filter {
    name   = "availability-zone"
    values = local.zones
  }
}

data "aws_ami" "ubuntu" {
  for_each    = var.instances
  most_recent = true
  owners      = ["099720109477"] # Canonical
  filter {
    name   = "name"
    values = ["ubuntu/images/hvm-ssd-gp3/ubuntu-noble-24.04-${each.value.arch}-server-*"]
  }
  filter {
    name   = "virtualization-type"
    values = ["hvm"]
  }
}

resource "tls_private_key" "ssh" {
  algorithm = "ED25519"
}

resource "local_sensitive_file" "ssh_key" {
  content         = tls_private_key.ssh.private_key_openssh
  filename        = "${path.module}/xo-ci-key.pem"
  file_permission = "0600"
}

resource "aws_key_pair" "ci" {
  key_name   = "xo-ci-key"
  public_key = tls_private_key.ssh.public_key_openssh
  tags       = { Name = "xo-ci-key" }
}

resource "aws_security_group" "ci" {
  name        = "xo-ci-ssh"
  description = "xo-ci: ssh from one address"
  vpc_id      = data.aws_vpc.default.id
  tags        = { Name = "xo-ci-ssh" }

  ingress {
    description = "ssh"
    from_port   = 22
    to_port     = 22
    protocol    = "tcp"
    cidr_blocks = [var.ssh_cidr]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

resource "aws_instance" "ci" {
  for_each                             = var.instances
  ami                                  = data.aws_ami.ubuntu[each.key].id
  instance_type                        = each.value.type
  key_name                             = aws_key_pair.ci.key_name
  subnet_id                            = data.aws_subnets.default.ids[0]
  vpc_security_group_ids               = [aws_security_group.ci.id]
  associate_public_ip_address          = true
  instance_initiated_shutdown_behavior = "terminate"

  root_block_device {
    volume_type           = "gp3"
    volume_size           = 60
    delete_on_termination = true
    tags                  = { Name = "xo-ci-${each.key}-root", project = "xo-ci" }
  }

  metadata_options {
    http_tokens = "required"
  }

  tags = { Name = "xo-ci-${each.key}" }
}

output "hosts" {
  value = { for k, i in aws_instance.ci : k => i.public_ip }
}

output "amis" {
  value = { for k, a in data.aws_ami.ubuntu : k => a.name }
}

output "key_file" {
  value = local_sensitive_file.ssh_key.filename
}
