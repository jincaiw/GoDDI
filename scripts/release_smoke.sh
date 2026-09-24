#!/usr/bin/env bash
set -euo pipefail

binary=${1:?usage: release_smoke.sh <binary> <version>}
expected_version=${2:?usage: release_smoke.sh <binary> <version>}
tmpdir=$(mktemp -d)
server_pid=""
port=$(python3 - <<'PY'
import socket
sock = socket.socket()
sock.bind(("127.0.0.1", 0))
print(sock.getsockname()[1])
sock.close()
PY
)
base_url="http://127.0.0.1:${port}"

cleanup() {
	if [[ -n "$server_pid" ]] && kill -0 "$server_pid" 2>/dev/null; then
		kill -TERM "$server_pid" 2>/dev/null || true
		wait "$server_pid" 2>/dev/null || true
	fi
	rm -rf "$tmpdir"
}
trap cleanup EXIT

version_output=$("$binary" version)
grep -Fq "GoDDI Version: ${expected_version}" <<<"$version_output"

export GODDI_SERVER_HTTP_ADDR="127.0.0.1:${port}"
export GODDI_SERVER_PUBLIC_URL="$base_url"
export GODDI_SERVER_DATA_DIR="$tmpdir/data"
export GODDI_DATABASE_DSN="$tmpdir/data/goddi.db"
export GODDI_SECURITY_JWT_SECRET="release-smoke-only-secret-2026"
export GODDI_ADMIN_USERNAME="release-admin"
export GODDI_ADMIN_PASSWORD="Release-Smoke-Admin-2026"
export GODDI_DNS_ENABLED=false
export GODDI_DHCP_ENABLED=false

start_server() {
	"$binary" serve >"$tmpdir/server.log" 2>&1 &
	server_pid=$!
	for _ in $(seq 1 45); do
		if curl -fsS "$base_url/health" >/dev/null 2>&1; then
			return 0
		fi
		if ! kill -0 "$server_pid" 2>/dev/null; then
			cat "$tmpdir/server.log"
			return 1
		fi
		sleep 1
	done
	cat "$tmpdir/server.log"
	return 1
}

login() {
	curl -fsS -X POST "$base_url/api/v1/auth/login" \
		-H 'Content-Type: application/json' \
		-d "{\"username\":\"$GODDI_ADMIN_USERNAME\",\"password\":\"$GODDI_ADMIN_PASSWORD\"}"
}

start_server
curl -fsS "$base_url/" | grep -q '<div id="app">'
session=$(login)
token=$(python3 -c 'import json,sys; print(json.load(sys.stdin)["data"]["token"])' <<<"$session")
csrf=$(python3 -c 'import json,sys; print(json.load(sys.stdin)["data"]["csrf_token"])' <<<"$session")
auth=(-H "Authorization: Bearer $token" -H "X-CSRF-Token: $csrf" -H 'Content-Type: application/json')
curl -fsS -X POST "$base_url/api/v1/dns/zones" "${auth[@]}" \
	-d '{"name":"release-smoke.test","type":"forward","enabled":true}' >/dev/null

kill -TERM "$server_pid"
wait "$server_pid"
server_pid=""

start_server
session=$(login)
token=$(python3 -c 'import json,sys; print(json.load(sys.stdin)["data"]["token"])' <<<"$session")
zones=$(curl -fsS "$base_url/api/v1/dns/zones" -H "Authorization: Bearer $token")
python3 -c 'import json,sys; data=json.load(sys.stdin)["data"]; items=data if isinstance(data,list) else data.get("items",[]); assert any(z["name"].rstrip(".")=="release-smoke.test" for z in items), "created zone did not survive restart"' <<<"$zones"

echo "Release binary smoke test passed (${expected_version}, clean database, embedded UI, login, persisted zone after restart)."
