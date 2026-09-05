# Production Review v0.1.1

## Plan

1. Inspect the working tree, release source completeness, and deployment entry points.
2. Run baseline Go tests and dependency checks before fixes.
3. Check desktop/mobile pages and production API scenarios on isolated test data.
4. Fix reproducible defects and add regression tests.
5. Run race tests, vet, frontend build, browser tests, Linux deployment checks.
6. Commit, build Linux release assets from the committed source, publish v0.1.1.

## Confirmed Findings

- Release source excluded `cmd/goddi` and frontend log pages due to broad ignore rules.
- DNS hourly charts grouped by full timestamps and compared mixed timestamp formats as text.
- Parent menu permissions did not filter DNS, DHCP, and IPAM children.
- Mobile login card could exceed viewport width.
- An unrelated `dist` directory could shadow embedded frontend assets.
- Frontend dependency audit found three high-severity transitive dependency advisories; compatible patches applied.
- DNS zone search sent the wrong filter name; breadcrumb category links pointed to nonexistent routes.
- Restricted API tokens could reach session-only account-security endpoints.
- Backup export converted SQL NULL to empty strings, breaking restored numeric DNS fields. NULL is preserved; legacy optional numeric fields are normalized.
- Restored DNS database state was not synchronized with live zones, filters, forwarders, or cache. Restore now reloads these services.
- Backup files were world-readable; new backups use owner-only permissions. Backup and restore operations are serialized.

Existing local changes to token scopes, TOTP encryption, and chart loading are included in this review.

## Scope

v0.1.1 remains a single-node SQLite release. Reserved extension APIs are not implemented features.
Real DHCP broadcast qualification requires an isolated LAN and is distinct from management API and packet-handler tests.

## Verification (2026-09-05)

- Baseline Go tests passed before fixes. Final `go test -race ./...`, `go vet ./...`, frontend type-check/build, and dependency audit passed (npm: zero known vulnerabilities).
- 48/48 integration scenarios passed against the Linux container: authentication, sessions, RBAC management, DNS records/forwarders/policies, DHCP scopes/leases, IPAM, settings, logs, cache, and explicit unsupported APIs.
- Three Playwright scenarios passed: 22 console routes without uncaught errors or server errors; language switch/mobile navigation; 320px login; CSRF rejection; scoped/single-use token isolation; backup download/delete/restore.
- The backup scenario additionally verified real DNS UDP and TCP answers on an isolated Linux container, before deletion and after restore. This exposed the NULL conversion defect that API-only checks missed.
- Added regression tests for nullable DNS backup fields, backup permissions, mixed-format hourly metrics, and embedded UI fallback, alongside existing local security tests.

## Limits And Operations

This is not a claim that every possible production workload or interaction has been exercised. No real-LAN DHCP broadcast/renewal qualification, long-duration soak/load test, live systemd host validation, or external DNSSEC interoperability certification was performed. Some packages have no dedicated unit tests; the 22-route browser sweep is not exhaustive CRUD coverage of every control.

Console backups are domain-data exports, not complete disaster-recovery images. Keep a stopped-service backup of the entire SQLite data directory and separately preserve configuration and encryption keys. Advanced DNS key/transfer tables and account/audit data are outside the current console export. Schedule restore in a maintenance window; startup/listener configuration still requires a restart.

The repository is public; `.trae` remains excluded from Git. Release source now includes the previously ignored executable entry point and log views.
