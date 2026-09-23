# GoDDI v0.20.0

This release keeps RFC 2136 dynamic updates from creating mixed-TTL RRsets.

## Changes

- When an update adds an RR with a different TTL, apply that TTL to every existing member of the same RRset in the update transaction.
- Re-adding an existing RDATA with a new TTL replaces the RRset TTL consistently.
- Journal each old and new RR when an RRset TTL changes; same-RDATA additions with the same TTL remain no-ops.
- Preserve subsecond timestamp precision for first-attempt replication backoff deadlines so second-level truncation cannot make a refused row immediately due.

## Verification

- `go test ./...`
- `go test -race ./internal/dns/zone ./internal/dns/transfer ./internal/dns/dynamic_update ./internal/dataplane ./internal/api/handler ./internal/dhcp ./internal/configver`
- `go vet ./...`
- `go build ./...`
- `git diff --check`

The change journal still does not preserve every RR type's metadata, so IXFR remains disabled and falls back to AXFR.
