# Import by list key to take over items already in the list. The import starts
# with no items; the next apply takes over the configured ones and leaves the
# rest alone.
terraform import quicknode_kv_list_items.treasury tracked-wallets
