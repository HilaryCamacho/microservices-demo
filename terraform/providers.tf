terraform {
  required_version = ">= 1.6"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 6.0"
    }
  }

  # El estado se guarda localmente (terraform.tfstate, excluido en .gitignore).
  # Para trabajo en equipo se recomienda migrar a un backend remoto en S3:
  #
  # backend "s3" {
  #   bucket       = "<bucket-de-estado>"
  #   key          = "online-boutique/terraform.tfstate"
  #   region       = "us-east-2"
  #   use_lockfile = true
  # }
}

provider "aws" {
  region = var.region

  default_tags {
    tags = {
      Project   = var.name
      ManagedBy = "terraform"
      Course    = "devops-tecmilenio"
    }
  }
}
