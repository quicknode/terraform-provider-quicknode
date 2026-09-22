# Changelog

## Unreleased

FEATURES:

* **New Resource:** `quicknode_endpoint`
* **New Resource:** `quicknode_endpoint_ip`
* **New Resource:** `quicknode_endpoint_domain_mask`
* **New Resource:** `quicknode_endpoint_referrer`
* **New Resource:** `quicknode_endpoint_jwt`
* **New Resource:** `quicknode_endpoint_request_filter`
* **New Resource:** `quicknode_endpoint_token`
* **New Resource:** `quicknode_endpoint_rate_limits`
* **New Resource:** `quicknode_endpoint_method_rate_limit`
* **New Data Source:** `quicknode_chains`
* **New Data Source:** `quicknode_endpoint`
* **New Data Source:** `quicknode_endpoints`

NOTES:

* `quicknode_endpoint` gains `security_options`, which decides what the endpoint
  enforces, and `ip_custom_header` for endpoints behind a proxy. A toggle left
  out of the configuration keeps whatever value the endpoint already has.
* Allowlist entries are imported by value rather than by the id the API
  assigned, so `terraform import quicknode_endpoint_ip.office 652052/203.0.113.7`
  needs nothing looked up first.
* Adding an allowlist entry while its toggle is disabled warns rather than
  fails. Building an allowlist before enabling enforcement is the safe order for
  an endpoint already serving traffic.
