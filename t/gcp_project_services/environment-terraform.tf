terraform {
  required_version = ">= 1.0"

  required_providers {
    google = {
      version = ">= 4.45"
      source  = "hashicorp/google"
    }
  }
}

provider "google" {}
