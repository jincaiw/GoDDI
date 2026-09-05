# GoDDI v0.1.2

## Highlights

- Reworked the web console with an Apple-inspired layout, typography, spacing, light/dark themes, and responsive mobile navigation.
- Added end-to-end UI regression coverage for DNS, DHCP, IPAM, administration, security, backups, pagination, locale switching, and responsive behavior.
- Fixed forwarder enable updates, user enable persistence, server-side pagination, DNS record search, stale create forms, confirmation dialog reopening, and live table header translation.
- Refreshed the embedded production web assets and verified the Linux single-file build.

## Verification

- Frontend production build passed.
- Go build and `go test -race ./...` passed.
- 17 Playwright workflow and route tests passed against the embedded production binary.
- Desktop and mobile visual checks passed at 1536x1024, 390x844, and 320x700.

This release remains a single-node SQLite deployment. DNS/DHCP listener behavior and external network integrations require qualification in the target production environment.
