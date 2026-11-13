#!/usr/bin/env bash
# ADR-0013 configuration replication (A) end-to-end acceptance.
#
# Proves the exact gap this feature closes: a DHCP scope authored on node-a
# appears on node-b automatically, and after node-a is gone and node-b promotes
# it serves that scope with NO manual scope rebuild -- the failover serving gap.
set -uo pipefail
cd "$(dirname "$0")/.."
TS=$(date -u +%Y%m%dT%H%M%SZ); OUT=reports; mkdir -p "$OUT"
PASS=0; FAIL=0; NV=0
ok(){ echo "PASS | $1"; PASS=$((PASS+1)); }
bad(){ echo "FAIL | $1"; FAIL=$((FAIL+1)); }
nv(){ echo "NOT_VERIFIED | $1"; NV=$((NV+1)); }
jget(){ python3 -c "import sys,json;d=json.load(sys.stdin);print($1)" 2>/dev/null; }
login(){ curl -s -X POST "http://127.0.0.1:$1/api/v1/auth/login" -H 'Content-Type: application/json' -d '{"username":"admin","password":"AdminPass-0123456789"}'; }
healthy(){ [ "$(docker inspect -f '{{.State.Health.Status}}' "$1" 2>/dev/null)" = healthy ]; }
waithealth(){ for _ in $(seq 1 45); do healthy "$1" && return 0; sleep 2; done; return 1; }
status_role(){ docker exec "$1" goddi ha status -c /etc/goddi/config.yaml 2>/dev/null | grep 'role in force' | awk '{print $4}'; }
has_scope(){ curl -s "http://127.0.0.1:$1/api/v1/dhcp/scopes" -H "Authorization: Bearer $2" | python3 -c "import sys,json;n='$3';d=json.load(sys.stdin).get('data') or [];print('yes' if any(s.get('name')==n for s in d) else 'no')" 2>/dev/null; }

echo "### bring the pair up with configuration replication + auto-failover"
docker compose -f docker-compose.yml -f docker-compose.configrepl.yml down -v --remove-orphans >/dev/null 2>&1
docker compose -f docker-compose.yml -f docker-compose.configrepl.yml up -d >/dev/null 2>&1
if waithealth node-a && waithealth node-b; then ok "pair healthy with config replication enabled on node-b"; else bad "pair did not come up"; exit 1; fi

RA=$(login 18181); TA=$(printf '%s' "$RA"|jget "d['data']['token']"); CA=$(printf '%s' "$RA"|jget "d['data']['csrf_token']")
HA=(-H "Authorization: Bearer $TA" -H "X-CSRF-Token: $CA" -H 'Content-Type: application/json')
RB=$(login 18182); TB=$(printf '%s' "$RB"|jget "d['data']['token']")

echo "### author a DHCP scope on node-a only"
SCOPE="crscope"
curl -s -X POST http://127.0.0.1:18181/api/v1/dhcp/scopes "${HA[@]}" \
  -d '{"name":"crscope","subnet":"10.77.3.0/24","start_ip":"10.77.3.120","end_ip":"10.77.3.140","lease_time":600}' -o /dev/null
sleep 1
[ "$(has_scope 18181 "$TA" "$SCOPE")" = "yes" ] && ok "node-a (author) has the scope" || bad "node-a scope create failed"

echo "### the scope must appear on node-b WITHOUT any manual step"
seen=no
for _ in $(seq 1 20); do
  [ "$(has_scope 18182 "$TB" "$SCOPE")" = "yes" ] && { seen=yes; break; }
  sleep 1
done
[ "$seen" = "yes" ] && ok "node-b received the scope via configuration replication (control DB)" || bad "node-b never received the scope"

# It must be in node-b's replicated control store AND in its DHCP data plane.
cb=$(docker exec node-b sh -c "sqlite3 /var/lib/goddi/goddi.db \"select count(*) from dhcp_scopes where name='crscope';\"" 2>/dev/null | tr -d '\r')
[ "${cb:-0}" -ge 1 ] && ok "scope present in node-b control database (goddi.db)" || bad "scope missing from node-b control DB (got '$cb')"
# Give the DHCP data-plane sync a moment to project it into the lease store.
lb=0
for _ in $(seq 1 15); do
  lb=$(docker exec node-b sh -c "sqlite3 /var/lib/goddi/leases.db \"select count(*) from dhcp_scopes where name='crscope';\"" 2>/dev/null | tr -d '\r')
  [ "${lb:-0}" -ge 1 ] && break; sleep 1
done
[ "${lb:-0}" -ge 1 ] && ok "scope projected into node-b DHCP data plane (leases.db)" || bad "scope not projected into node-b data plane (got '$lb')"

echo "### baseline DORA must succeed on node-a"
setmac(){ docker exec client sh -c "ip link set eth0 down; ip link set eth0 address $1; ip link set eth0 up" >/dev/null 2>&1; }
udhcpc(){ docker exec client sh -c 'rm -f /tmp/u.log; udhcpc -i eth0 -n -q -f -x hostname:h2c -t 8 -T 2 >/tmp/u.log 2>&1; echo EXIT=$?; cat /tmp/u.log'; }
ipof(){ printf '%s' "$1" | grep -o 'lease of [0-9.]*' | awk '{print $3}' | head -1; }
setmac 02:00:00:00:00:11
U1=$(udhcpc); IP1=$(ipof "$U1")
[ -n "${IP1:-}" ] && ok "client got a baseline lease from node-a: $IP1" || bad "no baseline lease (log: $(printf '%s' "$U1" | tr '\n' ' ' | head -c 140))"

echo "### kill node-a: node-b must promote and serve the replicated scope"
docker kill node-a >/dev/null 2>&1
promoted=no
for _ in $(seq 1 60); do
  [ "$(status_role node-b 2>/dev/null)" = "primary" ] && { promoted=yes; break; }
  sleep 2
done
[ "$promoted" = "yes" ] && ok "node-b promoted to primary after node-a was killed" || bad "node-b did not promote"
waithealth node-b || true
sleep 2
# A fresh DORA, no scope was recreated by hand, must be answered by node-b.
setmac 02:00:00:00:00:12
U2=$(udhcpc); IP2=$(ipof "$U2")
if [ -n "${IP2:-}" ]; then
  ok "promoted node-b served a lease from the replicated scope with no manual step: $IP2"
else
  bad "promoted node-b did not serve (log: $(printf '%s' "$U2" | tr '\n' ' ' | head -c 160))"
fi
# The leased address must come from the replicated pool (.120-.140).
case "$IP2" in
  10.77.3.1[2-4]?) ok "leased address $IP2 is inside the replicated pool 10.77.3.120-140";;
  *) [ -n "${IP2:-}" ] && bad "leased $IP2 is outside the replicated pool" || true;;
esac

echo "### restore the standard pair"
docker compose -f docker-compose.yml -f docker-compose.configrepl.yml down -v --remove-orphans >/dev/null 2>&1
docker compose -f docker-compose.yml up -d >/dev/null 2>&1
waithealth node-a && waithealth node-b && ok "restored the standard pair" || bad "failed to restore the standard pair"

printf '{"suite":"config-replication","timestamp":"%s","pass":%d,"fail":%d,"not_verified":%d}\n' "$TS" "$PASS" "$FAIL" "$NV" > "$OUT/config-replication-summary-$TS.json"
echo "----"; echo "CONFIG-REPLICATION PASS=$PASS FAIL=$FAIL NOT_VERIFIED=$NV"
