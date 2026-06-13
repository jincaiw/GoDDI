# GoDDI v0.1.0

The first public release of GoDDI provides an integrated, single-node DNS, DHCP, and IPAM management platform.

## Included

- Single Linux amd64 executable with embedded web UI and database migrations
- DNS zones, records, forwarding, cache, filtering, logs, and diagnostics
- DHCP scopes, leases, reservations, options, and logs
- IPAM spaces, subnets, addresses, allocation, and import/export
- RBAC, users, groups, API tokens, sessions, TOTP, and audit logs
- Full backup and restore, settings, health checks, and Prometheus metrics
- Responsive English and Simplified Chinese web console
- Hardened systemd service and Linux host-network Docker deployment

## Deployment Notes

- SQLite is the supported database for v0.1.0.
- Set a strong `GODDI_SECURITY_JWT_SECRET` before startup.
- Linux capabilities `CAP_NET_BIND_SERVICE` and `CAP_NET_RAW` are required when DNS or DHCP runs as a non-root user.
- See the English and Chinese README files for single-file and systemd deployment instructions.
