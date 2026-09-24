# GoDDI v0.21.0

This release makes DHCP-driven forward and reverse DNS publication atomic.

## Changes

- Publish a lease's A and PTR records in one DNS database transaction.
- Journal all changed RRsets together and advance the serial once per changed zone.
- If the reverse PTR RRset conflicts or any write/journal operation fails, roll back the A update and every serial change too.

## Verification

- `go test ./...`
- `go test -race ./internal/dhcp ./internal/dhcp/server`
- `go vet ./...`
- `go build ./...`
- `git diff --check`

Cross-database DHCP event delivery, replay under real network failures, and external relay/client interoperability remain outside this release's verification.
