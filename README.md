# GoDDI

English | [简体中文](README.zh-CN.md)

GoDDI is a compact, self-hosted DDI management platform that combines authoritative and recursive DNS, DHCP, and IP address management in one web console. The Linux release is a single executable with the web UI and database migrations embedded.

## Highlights

- DNS zones and records, recursive forwarding, cache, query logs, filtering, and a diagnostic client
- DHCP scopes, reservations, options, leases, and activity logs
- IPAM spaces, subnets, addresses, allocation status, and import/export
- Users, roles, groups, permissions, API tokens, TOTP, sessions, and audit logs
- DNS/DHCP/IPAM and policy backup and restore, system settings, health checks, and Prometheus metrics
- Responsive Vue 3 console with English and Simplified Chinese
- SQLite with WAL mode for a low-operations single-node deployment

## Demo

![GoDDI dashboard](docs/images/dashboard.png)

![DNS zone management](docs/images/dns-zones.png)

![Backup management](docs/images/backup.png)

## Version 0.1.2 Scope

GoDDI v0.1.3 is designed for a stable single-node deployment and supports SQLite only. DNS-over-TLS/HTTPS/QUIC configuration, DHCP high availability, SSO, clustering, and the application extension runtime are reserved APIs and return `501 Not Implemented` in this release.

## Quick Start

### Linux Single-File Deployment

Download the release binary:

```bash
curl -fL -o goddi \
  https://github.com/jincaiw/GoDDI/releases/download/v0.1.3/goddi-v0.1.3-linux-amd64
chmod +x goddi
sudo install -m 0755 goddi /usr/local/bin/goddi
```

GoDDI uses privileged DNS and DHCP ports. Run as root for a quick evaluation, or grant only the required Linux capabilities:

```bash
sudo setcap 'cap_net_bind_service,cap_net_raw=+ep' /usr/local/bin/goddi
export GODDI_SECURITY_JWT_SECRET="$(openssl rand -hex 32)"
export GODDI_SECURITY_ENCRYPTION_KEY="$(openssl rand -hex 32)"
export GODDI_ADMIN_USERNAME=admin
read -rsp 'Initial admin password (12+ characters, upper/lowercase and digits): ' GODDI_ADMIN_PASSWORD; echo
export GODDI_ADMIN_PASSWORD
goddi serve
```

Open `http://SERVER_IP:6080` and sign in with the initial administrator. The admin environment variables are used only when the database has no users; remove them after the first successful startup. Runtime data is stored in `./data` when no configuration file is supplied.

For a web-console-only evaluation without DNS or DHCP listeners:

```bash
export GODDI_SECURITY_JWT_SECRET="$(openssl rand -hex 32)"
export GODDI_SECURITY_ENCRYPTION_KEY="$(openssl rand -hex 32)"
export GODDI_ADMIN_USERNAME=admin
read -rsp 'Initial admin password: ' GODDI_ADMIN_PASSWORD; echo
export GODDI_ADMIN_PASSWORD
export GODDI_DNS_ENABLED=false
export GODDI_DHCP_ENABLED=false
goddi serve
```

### systemd Deployment

Create the service account and directories:

```bash
sudo useradd --system --home /var/lib/goddi --shell /usr/sbin/nologin goddi
sudo install -d -o goddi -g goddi -m 0750 /var/lib/goddi /var/log/goddi
sudo install -d -o root -g goddi -m 0750 /etc/goddi
sudo install -m 0755 goddi /usr/local/bin/goddi
sudo install -m 0644 deployments/systemd/goddi.service /etc/systemd/system/goddi.service
```

Store secrets outside the unit file:

```bash
sudo tee /etc/goddi/goddi.env >/dev/null <<EOF
GODDI_SECURITY_JWT_SECRET=$(openssl rand -hex 32)
GODDI_SECURITY_ENCRYPTION_KEY=$(openssl rand -hex 32)
GODDI_ADMIN_USERNAME=admin
GODDI_ADMIN_PASSWORD=CHANGE-ME-Before-Starting-123!
EOF
sudo chown root:goddi /etc/goddi/goddi.env
sudo chmod 0640 /etc/goddi/goddi.env
sudoedit /etc/goddi/goddi.env # Replace the example admin password before starting.
```

Optionally install and edit the full configuration:

```bash
sudo install -m 0640 -o root -g goddi config.yaml /etc/goddi/config.yaml
```

Enable the service:

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now goddi
sudo systemctl status goddi
sudo journalctl -u goddi -f
```

The provided unit runs as the unprivileged `goddi` user, grants only `CAP_NET_BIND_SERVICE` and `CAP_NET_RAW`, and applies systemd filesystem hardening.
After the first administrator is created, remove `GODDI_ADMIN_USERNAME` and `GODDI_ADMIN_PASSWORD` from `/etc/goddi/goddi.env` and restart the service.

## Configuration

The default HTTP port is `6080`; DNS listens on TCP/UDP `53`. Configuration is loaded from `/etc/goddi/config.yaml` when present and can be overridden with `GODDI_*` environment variables. Always replace `GODDI_SECURITY_JWT_SECRET` in production, and set `GODDI_SECURITY_ENCRYPTION_KEY` to use a dedicated key for encrypted TOTP secrets.

Useful endpoints:

- Web console: `http://SERVER_IP:6080`
- Health: `GET /health`
- Metrics: `GET /metrics`
- API: `/api/v1`

See [config.yaml](config.yaml) for the complete configuration template.

## Docker

DHCP requires broadcast traffic, so the included Compose file uses Linux host networking:

```bash
export GODDI_SECURITY_JWT_SECRET="$(openssl rand -hex 32)"
export GODDI_SECURITY_ENCRYPTION_KEY="$(openssl rand -hex 32)"
read -rsp 'Initial admin password: ' GODDI_ADMIN_PASSWORD; echo
export GODDI_ADMIN_PASSWORD
docker compose up -d --build
```

Host networking is supported by Docker Engine on Linux. For other platforms, disable DHCP and publish the required HTTP and DNS ports explicitly.

## Build and Test

Requirements: Go 1.26+, Node.js 22+, and npm.

```bash
cd web
npm ci
npm run build
cd ..
go test -race -cover ./...
go vet ./...
go build ./cmd/goddi
```

The frontend must be built before the Go binary because `web/dist` is embedded at compile time.

## Security Notes

- Console "full" backups cover DNS, DHCP, IPAM, DNS security policies, and database settings, not users, tokens, audit logs, external configuration, or encryption keys. For disaster recovery, stop the service and back up the complete data directory plus configuration and secrets. Do not copy a live SQLite database without its WAL or a SQLite-aware backup procedure.

- Persist both security keys securely across restarts and back them up separately. Changing the encryption key makes existing encrypted TOTP secrets unreadable. Do not regenerate keys during routine upgrades.
- See the [v0.1.1 production review](docs/production-review-v0.1.1.md) for the baseline verified coverage and remaining deployment qualification requirements.

- Put GoDDI behind HTTPS or a trusted private network; the built-in HTTP listener does not terminate TLS.
- Restrict `/metrics` and the management console with firewall or reverse-proxy policy.
- Back up `/var/lib/goddi` and test restore procedures regularly.
- Use API token expiration, IP restrictions, least-privilege roles, and TOTP for administrators.

## License

[MIT](LICENSE)
