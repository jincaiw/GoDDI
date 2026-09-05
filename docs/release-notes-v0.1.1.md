# GoDDI v0.1.1

Production-readiness fixes for the single-node SQLite release.

## Fixes

- Restore DNS runtime state after backups, preserve nullable numeric fields, support legacy optional numeric exports, and restrict backup file permissions.
- Enforce API token scopes alongside user permissions; require login sessions for account-security operations.
- Encrypt TOTP secrets with AES-GCM. Preserve `GODDI_SECURITY_ENCRYPTION_KEY` across restarts and upgrades; legacy plaintext secrets remain readable.
- Fix DNS hourly statistics, zone search, permission-filtered menus, invalid breadcrumb navigation, and narrow-screen login overflow.
- Include missing executable/log-view sources, avoid stale embedded frontend builds, and fix static asset fallback.
- Update vulnerable frontend dependencies and add browser/API regression checks to CI.

## Validation

Go race tests, vet, frontend build, npm audit, 48 API integration scenarios, 22 console routes, mobile/locale checks, and Linux DNS UDP/TCP backup-restore verification passed. See [the review report](production-review-v0.1.1.md) for exact coverage and limitations.

## Upgrade

Stop GoDDI and back up the complete data directory, configuration, and secrets before replacing the binary. Keep existing encryption/JWT keys. Console backups do not include account data, audit history, or all advanced DNS tables and are not complete disaster-recovery images. Listener settings require a restart after restoration.

Real-LAN DHCP and long-duration production load qualification remain deployment-specific. Reserved extension APIs still return 501.

Linux amd64 and arm64 single-file executables and SHA-256 checksums are provided.
