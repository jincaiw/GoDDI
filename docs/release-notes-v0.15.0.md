# GoDDI v0.15.0

This release adds a non-writing validation preview for CSV zone imports.

## Changes

- Accept `dry_run: true` on CSV zone import requests.
- Reuse the regular CSV parser and validation rules, then return `record_count` and per-type counts without opening a write transaction.
- Return validation errors before changing the zone; regular import behavior is unchanged.
- Add API regression coverage proving preview counts are accurate and no records are written.

## Verification

- `go test ./...`
- `go test -race ./internal/dns/zone ./internal/dns/transfer ./internal/dns/dynamic_update ./internal/dataplane ./internal/api/handler`
- `go vet ./...`
- `go build ./...`

Previews do not yet report collisions with existing RRsets or duplicate-record policy. IXFR continues to fall back to AXFR.
