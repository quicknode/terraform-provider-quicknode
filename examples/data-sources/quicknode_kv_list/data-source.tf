# Read a list that a Streams filter maintains.
data "quicknode_kv_list" "tracked_wallets" {
  key = "tracked-wallets"
}

output "tracked_wallet_count" {
  value = length(data.quicknode_kv_list.tracked_wallets.items)
}
