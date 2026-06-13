# GoDDI

English | [简体中文](README.zh-CN.md)

GoDDI is a compact, self-hosted DDI management platform that combines authoritative and recursive DNS, DHCP, and IP address management in one web console. The Linux release is a single executable with the web UI and database migrations embedded.

## Highlights

- DNS zones and records, recursive forwarding, cache, query logs, filtering, and a diagnostic client
- DHCP scopes, reservations, options, leases, and activity logs
- IPAM spaces, subnets, addresses, allocation status, and import/export
- Users, roles, groups, permissions, API tokens, TOTP, sessions, and audit logs
- Full-data backup and restore, system settings, health checks, and Prometheus metrics
- Responsive Vue 3 console with English and Simplified Chinese
- SQLite with WAL mode for a low-operations single-node deployment

## Demo

![GoDDI dashboard](docs/images/dashboard.png)

![DNS zone management](docs/images/dns-zones.png)

![Backup management](docs/images/backup.png)

## Version 0.1.0 Scope

GoDDI v0.1.0 is designed for a stable single-node deployment and supports SQLite only. DNS-over-TLS/HTTPS/QUIC configuration, DHCP high availability, SSO, clustering, and the application extension runtime are reserved APIs and return `501 Not Implemented` in this release.

## Quick Start

### Linux Single-File Deployment

Download the release binary:

```bash
curl -fL -o goddi \
  https://github.com/jincaiw/GoDDI/releases/download/v0.1.0/goddi-v0.1.0-linux-amd64
chmod +x goddi
sudo install -m 0755 goddi /usr/local/bin/goddi
```

GoDDI uses privileged DNS and DHCP ports. Run as root for a quick evaluation, or grant only the required Linux capabilities:

```bash
sudo setcap 'cap_net_bind_service,cap_net_raw=+ep' /usr/local/bin/goddi
export GODDI_SECURITY_JWT_SECRET="$(openssl rand -hex 32)"
export GODDI_ADMIN_USERNAME=admin
export GODDI_ADMIN_PASSWORD='replace-with-a-strong-password'
goddi serve
```

Open `http://SERVER_IP:6080` and sign in with the initial administrator. The admin environment variables are used only when the database has no users; remove them after the first successful startup. Runtime data is stored in `./data` when no configuration file is supplied.

For a web-console-only evaluation without DNS or DHCP listeners:

```bash
export GODDI_SECURITY_JWT_SECRET="$(openssl rand -hex 32)"
export GODDI_ADMIN_USERNAME=admin
export GODDI_ADMIN_PASSWORD='replace-with-a-strong-password'
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
GODDI_ADMIN_USERNAME=admin
GODDI_ADMIN_PASSWORD=replace-with-a-strong-password
EOF
sudo chown root:goddi /etc/goddi/goddi.env
sudo chmod 0640 /etc/goddi/goddi.env
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

The default HTTP port is `6080`; DNS listens on TCP/UDP `53`. Configuration is loaded from `/etc/goddi/config.yaml` when present and can be overridden with `GODDI_*` environment variables. Always replace `GODDI_SECURITY_JWT_SECRET` in production.

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
export GODDI_ADMIN_PASSWORD='replace-with-a-strong-password'
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

- Put GoDDI behind HTTPS or a trusted private network; the built-in HTTP listener does not terminate TLS.
- Restrict `/metrics` and the management console with firewall or reverse-proxy policy.
- Back up `/var/lib/goddi` and test restore procedures regularly.
- Use API token expiration, IP restrictions, least-privilege roles, and TOTP for administrators.

## License

[MIT](LICENSE)
