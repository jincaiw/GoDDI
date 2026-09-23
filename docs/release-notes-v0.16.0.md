# GoDDI v0.16.0

This release makes CSV zone imports idempotent and expands their preflight checks.

## Changes

- CSV preview reports total rows, per-type counts, new records, and unchanged records.
- Skip exact records already in the zone; repeated identical rows in one file are written once.
- Reject identical RDATA supplied with a different TTL instead of silently ignoring the TTL difference.
- Detect CNAME conflicts against existing records and within the incoming file before any writes.
- Validate TTL bounds and record values with the same DNS validation rules used by API record writes.
- Keep preview and import classification on the same SQLite snapshot; rejected imports leave no partial records.

## Verification

- `go test ./...`
- `go test -race ./internal/dns/zone ./internal/dns/transfer ./internal/dns/dynamic_update ./internal/dataplane ./internal/api/handler`
- `go vet ./...`
- `go build ./...`

Other record-set consistency rules still need type-by-type review. IXFR continues to fall back to AXFR.
