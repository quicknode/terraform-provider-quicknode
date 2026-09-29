# Read a value that a Streams filter writes, such as the last block processed.
data "quicknode_kv_value" "last_block" {
  key = "last-processed-block"
}

output "last_processed_block" {
  value = tonumber(data.quicknode_kv_value.last_block.value)
}
