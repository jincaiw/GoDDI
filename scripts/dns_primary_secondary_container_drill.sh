#!/usr/bin/env bash
# DNS 方案A（标准主从）两容器 + dig 现场版演练。
# 起两个真实 DNS 容器（复用产品 transfer 包：primary=AXFRHandler/TSIG，
# secondary=SecondarySync 经 TCP 拉取），用独立 dig 客户端容器实测复制与
# "停掉 primary 后 secondary 仍应答"的读容灾。输出 PASS/FAIL 与 DRILL: OK。
set -uo pipefail
cd "$(dirname "$0")/.."
NET=dnsdrill-net
IMG=goddi:dnsdrill
PASS=0; FAIL=0
ok(){ echo "PASS | $1"; PASS=$((PASS+1)); }
bad(){ echo "FAIL | $1"; FAIL=$((FAIL+1)); }

echo "== build drill binary + image =="
GOOS=linux GOARCH="$(go env GOARCH)" CGO_ENABLED=0 go build -mod=readonly -o .dnsdrill-bin ./scripts/dns_drill_server || { echo "build failed"; exit 1; }
docker build -q -f Dockerfile.dnsdrill -t "$IMG" . >/dev/null || { echo "docker build failed"; exit 1; }

cleanup(){
  docker rm -f dnsdrill-primary dnsdrill-secondary dnsdrill-client >/dev/null 2>&1 || true
  docker network rm "$NET" >/dev/null 2>&1 || true
}
trap cleanup EXIT
cleanup

SECRET=$(openssl rand -base64 32 | tr -d '\n')
docker network create "$NET" >/dev/null

echo "== start primary + secondary containers =="
docker run -d --name dnsdrill-primary --network "$NET" "$IMG" \
  --mode primary --db /data/dns.db --zone drill.test. --record 192.0.2.10 \
  --tsig-name xfr-key. --tsig-secret "$SECRET" >/dev/null
docker run -d --name dnsdrill-secondary --network "$NET" "$IMG" \
  --mode secondary --db /data/dns.db --zone drill.test. --primary dnsdrill-primary:53 \
  --tsig-name xfr-key. --tsig-secret "$SECRET" >/dev/null
docker run -d --name dnsdrill-client --network "$NET" alpine:3.24 sh -c 'apk add -q bind-tools >/dev/null 2>&1; sleep 600' >/dev/null

digq(){ docker exec dnsdrill-client dig +short +time=1 +tries=1 "@$1" www.drill.test. A 2>/dev/null | tr -d '\r' | head -1; }

# Install dig in the client container, retrying until it is actually present
# (the first apk attempt can race the container network coming up).
installed=0
for _ in $(seq 1 120); do
  if docker exec dnsdrill-client sh -c 'command -v dig >/dev/null 2>&1'; then installed=1; break; fi
  docker exec dnsdrill-client sh -c 'apk add -q bind-tools >/dev/null 2>&1' >/dev/null 2>&1 || true
  sleep 1
done
[ "$installed" = 1 ] || echo "WARN: dig install did not complete"

# Wait for primary to answer.
p=""
for _ in $(seq 1 40); do p=$(digq dnsdrill-primary); [ "$p" = "192.0.2.10" ] && break; sleep 0.5; done
[ "$p" = "192.0.2.10" ] && ok "primary (dig) answers www.drill.test. A = $p" || bad "primary dig answer = '$p'"

# Wait for the secondary to pull the zone via TSIG AXFR.
s=""
for _ in $(seq 1 60); do s=$(digq dnsdrill-secondary); [ "$s" = "192.0.2.10" ] && break; sleep 1; done
[ "$s" = "192.0.2.10" ] && ok "secondary (dig) mirrors www.drill.test. A = $s (TSIG AXFR)" || bad "secondary dig answer = '$s'"

# Fail-closed sanity: an unsigned AXFR must be refused by the primary.
rc=$(docker exec dnsdrill-client sh -c "dig +time=1 +tries=1 @dnsdrill-primary drill.test. AXFR 2>&1" | grep -ciE "communications error|refused|failed|no servers" || true)
[ "${rc:-0}" -gt 0 ] && ok "primary refuses unsigned AXFR (TSIG fail-closed)" || bad "primary served unsigned AXFR"

# Read failover: stop the primary, the secondary must keep answering.
docker stop dnsdrill-primary >/dev/null
sleep 1
pd=$(digq dnsdrill-primary)
[ -z "$pd" ] && ok "primary down: primary no longer answers" || bad "primary still answers after stop ($pd)"
sd=$(digq dnsdrill-secondary)
[ "$sd" = "192.0.2.10" ] && ok "secondary still answers after primary down (read failover)" || bad "secondary stopped answering ($sd)"

echo "===================================================="
echo "PASS=$PASS FAIL=$FAIL"
if [ "$FAIL" -eq 0 ]; then echo "DRILL: OK"; else echo "DRILL: FAIL"; fi
[ "$FAIL" -eq 0 ]
