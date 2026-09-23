# GoDDI v0.13.0

This release makes DNS record imports fail clearly when the input cannot be parsed or preserved.

## Changes

- Reject malformed CSV rows, invalid numeric fields, and unsupported record types instead of silently skipping them.
- Validate the full CSV before writing, so a rejected import cannot leave a partial zone update.
- Reject unknown or unrepresentable zone-file records rather than reporting a successful partial import.
- Reject NAPTR zone-file imports while the record model cannot preserve all NAPTR RDATA fields.
- Add regression coverage for malformed CSV, mixed valid/unsupported rows, and lossy NAPTR input.

## Verification

- `go test ./...`
- `go test -race ./internal/dns/zone ./internal/dns/transfer ./internal/dns/dynamic_update ./internal/dataplane`
- `go vet ./...`
- `go build ./...`

CSV dry-run/preflight and full lossless support for additional record types remain open. IXFR continues to fall back to AXFR.
