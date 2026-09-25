terraform {
  required_providers {
    quicknode = {
      source  = "quicknode/quicknode"
      version = "~> 0.3.0"
    }
  }
}

# Reads the API key from QUICKNODE_API_KEY. Set api_key here to override it.
provider "quicknode" {}
