# GoDDI v0.12.0

This release fixes loss of CAA record metadata across DNS management and import/export paths.

## Changes

- Persist and return CAA `flag` and `tag` fields in single and batch record creation, record reads, and record listings.
- Preserve existing CAA metadata when updating only the record value.
- Include CAA metadata in zone change history so delete/add journal entries retain the original RDATA fields.
- Preserve CAA metadata through zone-file and CSV import/export.
- Reject CAA flags outside the wire-format octet range (0–255).
- Add reversible control-plane and zone data-plane migrations for the history fields.

## Verification

- `go test ./...`
- `go test -race ./internal/dns/zone ./internal/dns/transfer ./internal/dns/dynamic_update ./internal/dataplane`
- `go vet ./...`
- `go build ./...`

IXFR remains disabled and falls back to AXFR pending complete journaling across every zone mutation path and protocol-level delta-transfer coverage.
