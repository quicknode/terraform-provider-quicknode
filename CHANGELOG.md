# Changelog

## 0.3.0 (Unreleased)

BREAKING CHANGES:

* `quicknode_endpoint.status` and `quicknode_endpoint.multichain` are required. Their
  defaults applied on import too, so importing a paused or multichain endpoint without
  declaring them planned to resume it or turn multichain off.
* `tags` on `data.quicknode_endpoint` and `data.quicknode_endpoints`, and the
  `labels`, `networks`, `statuses` and `tag_labels` filters on `data.quicknode_endpoints`,
  are sets instead of lists.

BUG FIXES:

* Importing an IP address, domain mask or referrer while its security toggle is off
  reports the disabled toggle. The Admin API hides those entries while it is off.
* Creating one that already exists while its toggle is off explains that it is hidden
  and how to import it.

## 0.2.0 (September 25, 2026)

BREAKING CHANGES:

* `safe_http_url` and `safe_wss_url` carry `REPLACE_WITH_TOKEN` in place of `TOKEN`.
* `quicknode_endpoint` no longer has `tokens`, `http_url_with_token` or
  `wss_url_with_token`. Adding or removing a token changed them, so they went stale
  after every apply that touched a `quicknode_endpoint_token`. Read working URLs from
  `data.quicknode_endpoint_urls` or `quicknode_endpoint_token.http_url_with_token`.
* `quicknode_endpoint.security_options` no longer has `request_filters`. The Admin API
  sets it when a filter exists, so it went stale after every apply that created one.
  `data.quicknode_endpoint` still reports it.
* Removing `label` from a `quicknode_endpoint` clears the endpoint's label.

FEATURES:

* **New Data Source:** `quicknode_endpoint_urls`, with `safe_multichain_urls` and
  `multichain_urls_with_token` for every network a multichain endpoint serves.
* `quicknode_endpoint_token` has `http_url_with_token` and `wss_url_with_token`,
  carrying that token.

## 0.1.0

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
* `quicknode_endpoint.label` cannot be cleared once set. Removing the attribute
  leaves the endpoint's label in place.
* `quicknode_endpoint_jwt.kid` is required, and matches the `kid` header of the
  tokens signed with the corresponding private key.
