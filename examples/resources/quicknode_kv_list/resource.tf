variable "watched_wallets" {
  type = set(string)
}

# Terraform owns the whole list. A Streams filter can read it with
# qnLib.qnContainsListItems("watched-wallets", [...]).
resource "quicknode_kv_list" "watched_wallets" {
  key   = "watched-wallets"
  items = var.watched_wallets
}
