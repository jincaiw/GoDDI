# GoDDI v0.17.0

This release rejects CSV imports that would create inconsistent TTLs within a DNS RRset.

## Changes

- Check that all records with the same owner and type use one TTL, including records already in the destination zone.
- Apply the same RRset TTL preflight to CSV preview and import.
- Reject conflicts before writing any rows, preserving the existing zone data.

## Verification

- `go test ./...`
- `go test -race ./internal/dns/zone ./internal/dns/transfer ./internal/dns/dynamic_update ./internal/dataplane ./internal/api/handler`
- `go vet ./...`
- `go build ./...`
- `git diff --check`

The RRset TTL check currently covers CSV imports. Other record creation, update, and import paths still need review. IXFR continues to fall back to AXFR.
