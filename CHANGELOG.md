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

* `safe_http_url` and `safe_wss_url` carry the literal `TOKEN` where the credential
  belongs, so `replace(url, "TOKEN", token)` rebuilds a working address on every chain.
* `quicknode_endpoint.security_options` decides what the endpoint enforces. A toggle
  left out of the configuration keeps the value the endpoint already has.
* `quicknode_endpoint.ip_custom_header` names the header an endpoint behind a proxy
  reads the caller's IP address from.
* Allowlist entries are imported by value, as `123456/203.0.113.7`.
* Adding an allowlist entry while its toggle is disabled warns and succeeds, so an
  allowlist can be built before enforcement is turned on.
* `quicknode_endpoint.label` cannot be cleared once set, so removing the attribute
  leaves the endpoint's label in place.
* `quicknode_endpoint_jwt` takes `kid` as an input. The Admin API requires it when
  the signing key is registered.
