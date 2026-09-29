variable "archive_secret_key" {
  type      = string
  sensitive = true
}

variable "archive_sas_token" {
  type      = string
  sensitive = true
}

variable "database_password" {
  type      = string
  sensitive = true
}

variable "kafka_password" {
  type      = string
  sensitive = true
}

resource "quicknode_stream" "transfers" {
  name    = "usdc-transfers"
  network = "ethereum-mainnet"
  dataset = "receipts"
  region  = "usa_east"
  status  = "active"

  # The filter is plain source; the provider encodes it for the API.
  filter_function = file("${path.module}/filter.js")

  # Stay behind the newest block so reorganized blocks are never delivered.
  keep_distance_from_tip = 3

  destination = {
    webhook = {
      url     = "https://ingest.example.com/quicknode"
      headers = { "X-Source" = "quicknode-streams" }
    }
  }

  # Every batch is also delivered to each of these. The stream moves on only
  # after every destination accepts a batch.
  extra_destinations = [
    {
      s3 = {
        bucket        = "chain-archive"
        region        = "us-east-1"
        object_prefix = "usdc-transfers/"
        compression   = "gzip"
        access_key    = "AKIAIOSFODNN7EXAMPLE"
        secret_key    = var.archive_secret_key
      }
    },
    {
      azure = {
        storage_account = "chainarchive"
        container       = "usdc-transfers"
        file_type       = ".parquet"
        sas_token       = var.archive_sas_token
      }
    },
    {
      postgres = {
        host       = "db.example.com"
        database   = "chain"
        table_name = "usdc_transfers"
        username   = "streams"
        password   = var.database_password
      }
    },
    {
      kafka = {
        bootstrap_servers = "broker-1.example.com:9092,broker-2.example.com:9092"
        topic_name        = "usdc-transfers"
        protocol          = "sasl_ssl"
        mechanisms        = "SCRAM-SHA-512"
        username          = "streams"
        password          = var.kafka_password
      }
    },
  ]
}

# Quicknode signs each webhook request with this token, so the receiver can
# check it came from Quicknode.
output "webhook_security_token" {
  value     = quicknode_stream.transfers.destination.webhook.security_token
  sensitive = true
}

# A backfill of a fixed block range. It stops as completed after end_range,
# and any later change replaces it.
resource "quicknode_stream" "backfill" {
  name        = "usdc-transfers-backfill"
  network     = "ethereum-mainnet"
  dataset     = "receipts"
  region      = "usa_east"
  status      = "active"
  start_range = 21000000
  end_range   = 21100000

  dataset_batch_size = 10

  filter_function = file("${path.module}/filter.js")

  destination = {
    webhook = {
      url         = "https://ingest.example.com/quicknode/backfill"
      compression = "gzip"
    }
  }
}
