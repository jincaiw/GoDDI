# GoDDI v0.19.0

This release applies DNS RRset TTL consistency checks to expiring records and automatic DNS projections.

## Changes

- Ignore expired records when checking live RRset TTL consistency and classifying CSV imports.
- Reject automatic PTR creation when it would conflict with the reverse RRset TTL; the forward record and PTR remain in one transaction.
- Store automatic PTR owners as fully qualified names relative to their reverse zone, fixing records that previously could not answer under the reverse-zone owner.
- Reject DHCP forward A and reverse PTR writes that would conflict with an enabled RRset's TTL.
- Check control-plane configuration releases against data-plane-authored RRsets before replacing records.
- Reject transferred AXFR snapshots with inconsistent RRset TTLs before replacing the secondary's last known-good data.

## Verification

- `go test ./...`
- `go test -race ./internal/dns/zone ./internal/dns/transfer ./internal/dns/dynamic_update ./internal/dataplane ./internal/api/handler ./internal/dhcp`
- `go vet ./...`
- `go build ./...`
- `git diff --check`

DHCP forward and reverse projections still use separate transactions, so the pair is not atomic. Catalog writes and TTL normalization for dynamic updates remain under review. IXFR continues to fall back to AXFR.
