# Relay on AWS (Part 2.3): one t3.micro running Docker Compose, an S3 bucket for
# profile pictures, and guard-rails that keep the bill at $0. See deploy/README.md.
#
#   make infra-init     once
#   make infra-plan     what would change?
#   make infra-apply    create / update everything
#   make infra-destroy  delete everything (do this when you're done for the day)
terraform {
  required_version = ">= 1.10"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 6.0"
    }
  }
}

provider "aws" {
  region = var.region
  # Every resource gets these tags: Cost Explorer can show "what does relay cost?"
  # and you can find anything left behind with the Tag Editor.
  default_tags {
    tags = {
      project    = "relay"
      managed-by = "terraform"
    }
  }
}
