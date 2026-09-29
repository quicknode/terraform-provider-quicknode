# Look up a stream created outside Terraform.
data "quicknode_stream" "transfers" {
  id = "0b4a6f8e-3c1d-4e2a-9f7b-5d6c8e1a2b3c"
}

# The last block the stream delivered. It moves while the stream runs.
output "transfers_sequence" {
  value = data.quicknode_stream.transfers.sequence
}

output "transfers_state" {
  value = data.quicknode_stream.transfers.status
}
