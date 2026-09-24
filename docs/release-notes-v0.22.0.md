# GoDDI v0.22.0

This release makes DNS CSV import previews actionable by reporting all record conflicts in the file and target zone.

## Changes

- CSV dry-run responses include `valid` and a list of conflicts with the CSV row, owner, record type, stable code, and explanation.
- The preview reports all RRset TTL mismatches, CNAME exclusivity conflicts, and multiple CNAME targets found in the existing zone or incoming file.
- Preview counts distinguish unchanged records from rows blocked by conflicts.
- Actual CSV imports use the same conflict checks and reject the full batch before writes when any conflict exists.

## Verification

- `go test ./internal/dns/zone ./internal/api/handler`
- `git diff --check`

The broader v1.2 assessment remains in progress. Cross-database durable event delivery, HA fencing, recovery drills, and IXFR re-enablement are not covered by this release.
