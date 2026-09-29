# A setting a Streams filter reads with qnLib.qnGetValue("alert-threshold").
resource "quicknode_kv_value" "alert_threshold" {
  key   = "alert-threshold"
  value = "1000000"
}

# Values are strings; use jsonencode for structured settings.
resource "quicknode_kv_value" "alert_config" {
  key = "alert-config"
  value = jsonencode({
    min_value_usd = 50000
    tokens        = ["USDC", "USDT"]
  })
}
