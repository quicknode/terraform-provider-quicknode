# A list that a Streams filter also adds to. Terraform tracks only these items
# and leaves the filter's items in place.
resource "quicknode_kv_list_items" "treasury" {
  list_key = "tracked-wallets"
  items = [
    "0xd8dA6BF26964aF9D7eEd9e03E53415D37aA96045",
    "0x71C7656EC7ab88b098defB751B7401B5f6d8976F",
  ]
}
