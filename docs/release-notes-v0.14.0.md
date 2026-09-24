# GoDDI v0.14.0

This release preserves NAPTR records and supports zone-relative CSV imports.

## Changes

- Preserve all NAPTR RDATA fields: flags, service, regexp, and replacement. Order and preference use the existing priority and weight fields.
- Validate NAPTR order, preference, and presentation data on API and CSV writes.
- Normalize legacy NAPTR rows that stored only a replacement name, preserving their previous empty-string wire behavior.
- Export CSV owner names relative to their zone and resolve them against the destination zone during import.
- Reject CSV and zone-file owners outside the selected zone before writing any records.
- Add round-trip regression coverage for NAPTR API answers, CSV, and zone files, plus import boundary checks.

## Verification

- `go test ./...`
- `go test -race ./internal/dns/zone ./internal/dns/transfer ./internal/dns/dynamic_update ./internal/dataplane`
- `go vet ./...`
- `go build ./...`

CSV dry-run/preflight remains open. IXFR continues to fall back to AXFR.
