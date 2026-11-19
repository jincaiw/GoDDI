#!/usr/bin/env bash
# H2 split-plane acceptance: management (role=control) and DNS (role=dns) run
# as separate processes/containers over the same control DB. The proof is that
# DNS keeps answering from its own dnsdata.db while control is stopped.
# This is Docker H2 evidence, not physical-host or production acceptance.
set -uo pipefail
cd "$(dirname "$0")/.."
TS=$(date -u +%Y%m%dT%H%M%SZ); OUT=reports; mkdir -p "$OUT"
PASS=0; FAIL=0; NV=0
ok(){ echo "PASS | $1"; PASS=$((PASS+1)); }
bad(){ echo "FAIL | $1"; FAIL=$((FAIL+1)); }
nv(){ echo "NOT_VERIFIED | $1"; NV=$((NV+1)); }
jget(){ python3 -c "import sys,json;d=json.load(sys.stdin);print($1)" 2>/dev/null; }
q(){ docker exec split-client sh -c "nslookup $1 dns 2>/dev/null | grep '^Address: ' | grep -v ':53' | tail -1 | cut -d ' ' -f2" | tr -d '\r' | head -1; }

echo "### start split-plane stack"
docker compose -f docker-compose.splitplane.yml down -v --remove-orphans >/dev/null 2>&1
docker compose -f docker-compose.splitplane.yml up -d >/dev/null 2>&1
ready=no
for _ in $(seq 1 40); do
  [ "$(docker inspect -f "{{.State.Health.Status}}" split-control 2>/dev/null)" = healthy ] && { ready=yes; break; }
  sleep 1
done
[ "$ready" = yes ] && ok "split-control healthy on management port only" || { bad "split-control did not become healthy"; exit 1; }
docker exec split-control sh -c 'netstat -lntup 2>/dev/null | grep -q ":53 " && exit 1 || exit 0' && ok "control process does not bind DNS" || bad "control process bound DNS"
docker exec split-dns sh -c 'netstat -lntp 2>/dev/null | grep -q ":6080 " && exit 1 || exit 0' && ok "dns process does not bind management API" || bad "dns process bound management API"

echo "### publish zone through control and resolve through dns"
RA=$(curl -s -X POST http://127.0.0.1:18191/api/v1/auth/login -H 'Content-Type: application/json' -d '{"username":"admin","password":"AdminPass-0123456789"}')
TOK=$(printf "%s" "$RA" | jget "d['data']['token']"); CS=$(printf "%s" "$RA" | jget "d['data']['csrf_token']")
H=(-H "Authorization: Bearer $TOK" -H "X-CSRF-Token: $CS" -H "Content-Type: application/json")
Z=$(curl -s -X POST http://127.0.0.1:18191/api/v1/dns/zones "${H[@]}" -d '{"name":"split-role.test.","type":"primary"}' | jget "d['data']['id']")
[ -n "$Z" ] && ok "zone created through role=control API" || { bad "zone create failed"; exit 1; }
curl -s -X POST "http://127.0.0.1:18191/api/v1/dns/zones/$Z/records" "${H[@]}" -d '{"name":"www","type":"A","ttl":60,"value":"192.0.2.177"}' -o /dev/null
answer=""
for _ in $(seq 1 20); do answer=$(q www.split-role.test.); [ "$answer" = "192.0.2.177" ] && break; sleep 1; done
[ "$answer" = "192.0.2.177" ] && ok "role=dns serves published record ($answer)" || bad "dns did not answer published record (got '$answer')"

echo "### control outage does not stop DNS"
docker stop split-control >/dev/null 2>&1
sleep 2
answer2=$(q www.split-role.test.)
[ "$answer2" = "192.0.2.177" ] && ok "DNS keeps answering while control process is down" || bad "DNS stopped when control was down (got '$answer2')"

echo "### control recovery"
docker start split-control >/dev/null 2>&1
ready=no
for _ in $(seq 1 40); do [ "$(docker inspect -f "{{.State.Health.Status}}" split-control 2>/dev/null)" = healthy ] && { ready=yes; break; }; sleep 1; done
[ "$ready" = yes ] && ok "control recovered after restart" || bad "control did not recover"
answer3=$(q www.split-role.test.)
[ "$answer3" = "192.0.2.177" ] && ok "DNS still serves after control recovery" || bad "DNS answer changed after control recovery (got '$answer3')"

printf '{"suite":"split-plane","timestamp":"%s","pass":%d,"fail":%d,"not_verified":%d}\n' "$TS" "$PASS" "$FAIL" "$NV" > "$OUT/split-plane-summary-$TS.json"
docker compose -f docker-compose.splitplane.yml down -v --remove-orphans >/dev/null 2>&1
echo "----"; echo "SPLIT-PLANE PASS=$PASS FAIL=$FAIL NOT_VERIFIED=$NV"
[ "$FAIL" -eq 0 ]
