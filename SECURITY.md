# Security Policy

## Reporting a vulnerability

Do not open a public issue for security problems.

Report vulnerabilities in this provider to **security@quicknode.com**, or through
the security contact listed at https://www.quicknode.com/security. Include the
provider version, a description of the issue, and reproduction steps.

## Credentials and Terraform state

Terraform writes every attribute it reads into state, including attributes marked
sensitive. For this provider that includes endpoint auth tokens and credentialed
endpoint URLs.

Marking an attribute sensitive keeps it out of CLI output and plan diffs. It does
not encrypt it, and it does not keep it out of the state file.

Use [encrypted remote state](https://developer.hashicorp.com/terraform/language/state/sensitive-data)
with access controls, treat state files as credential material, and never commit
them to version control.

## Supported versions

Security fixes are applied to the latest released minor version.
