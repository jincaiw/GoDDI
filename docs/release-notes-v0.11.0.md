# GoDDI v0.11.0

This release hardens authoritative DNS record changes, zone transfers, and DHCP-driven DNS publication.

## Changes

- Keep DNS record changes, SOA serial advancement, and IXFR history in one transaction for the covered management, import, catalog, DHCP projection, and dynamic update paths.
- Store zone change history in the zone data-plane database so split deployments use the same database transaction as zone records.
- Return AXFR for IXFR requests while complete journal coverage and RFC 1982 serial arithmetic remain under development.
- Fail closed when an AXFR snapshot contains unsupported or malformed records, invalid SOA framing, or an incomplete database read.
- Validate secondary AXFR framing, class, owner scope, and representable TXT character strings before replacing the stored zone.
- Keep authoritative snapshots consistent across database reads, expiry handling, and explicit TTL values.
- Notify affected primary zones after committed DNS changes and refresh the in-memory authoritative snapshot after committed dynamic updates.
- Preserve the exact TXT character-string boundaries supported by the database; reject updates that cannot be represented without data loss.

## Verification

- `go test ./...`
- `go build ./...`
- `go vet ./...`
- `go test -race ./internal/dataplane ./internal/dhcp/... ./internal/dns/... ./internal/configver`
- Frontend typecheck and production build
- Targeted split-plane DHCP-to-DNS outbox and ACK-to-resolvable latency regressions

All listed checks passed on the release branch before publication.

## Scope

IXFR remains disabled and falls back to AXFR until every zone mutation path maintains a complete journal and protocol-level delta-transfer coverage passes. Hardware power-loss durability, real DHCP relay/client interoperability, cross-host fencing and partition behavior, and capacity/long-run qualification require target-environment validation and are not claimed by this release.
