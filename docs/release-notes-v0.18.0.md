# GoDDI v0.18.0

This release enforces consistent TTLs across enabled DNS records in an RRset.

## Changes

- Check RRset TTL consistency for single and batch record creation, record updates, and zone-file imports.
- Run the checks in the same transaction as the corresponding write.
- Roll back a complete batch or zone-file import when any RRset has inconsistent TTLs.
- Check updates only when the record's owner, type, TTL, or enabled state changes.

## Verification

- `go test ./...`
- `go test -race ./internal/dns/zone ./internal/dns/transfer ./internal/dns/dynamic_update ./internal/dataplane ./internal/api/handler`
- `go vet ./...`
- `go build ./...`
- `git diff --check`

Other bulk producers, including catalog and DHCP projections, still need review for this invariant. IXFR continues to fall back to AXFR.
