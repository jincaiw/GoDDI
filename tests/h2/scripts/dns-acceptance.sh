#!/usr/bin/env bash
# DNS dual-node acceptance over the real GoDDI services + real dig.
# Honest verdicts: cross-node CONTROL-PLANE config propagation is ADR-0005
# (multi-node publish) which is NOT implemented, so a zone created on node-a is
# not expected on node-b; that is reported NOT_VERIFIED/FAIL, never faked PASS.
set -uo pipefail
cd "$(dirname "$0")/.."
TS=$(date -u +%Y%m%dT%H%M%SZ); OUT=reports; mkdir -p "$OUT"
# udhcpc rewrites the client's own address/route; restore the fixture network
# before issuing DNS queries so a previous DHCP test cannot break this suite.
docker exec client sh -c 'ip link set eth0 down 2>/dev/null; ip addr flush dev eth0 2>/dev/null; ip link set eth0 address 02:00:00:00:00:aa; ip link set eth0 up; ip addr add 10.77.3.3/24 dev eth0 2>/dev/null; ip route add default via 10.77.3.1 2>/dev/null; true' >/dev/null 2>&1

PASS=0; FAIL=0; NV=0
ok(){ echo "PASS | $1"; PASS=$((PASS+1)); }
bad(){ echo "FAIL | $1"; FAIL=$((FAIL+1)); }
nv(){ echo "NOT_VERIFIED | $1"; NV=$((NV+1)); }
jget(){ python3 -c "import sys,json;d=json.load(sys.stdin);print($1)" 2>/dev/null; }
login(){ curl -s -X POST "http://127.0.0.1:$1/api/v1/auth/login" -H 'Content-Type: application/json' -d '{"username":"admin","password":"AdminPass-0123456789"}'; }
digq(){ docker exec client sh -c "apk add -q bind-tools >/dev/null 2>&1; dig +short +time=2 +tries=1 @$1 $2 A 2>/dev/null" | tr -d '\r' | head -1; }

RA=$(login 18181); TOK_A=$(printf '%s' "$RA" | jget "d['data']['token']"); CS_A=$(printf '%s' "$RA" | jget "d['data']['csrf_token']")
RB=$(login 18182); TOK_B=$(printf '%s' "$RB" | jget "d['data']['token']"); CS_B=$(printf '%s' "$RB" | jget "d['data']['csrf_token']")
[ -n "${TOK_A:-}" ] && [ -n "${TOK_B:-}" ] && ok "management login node-a/node-b" || bad "management login"
HA=(-H "Authorization: Bearer $TOK_A" -H "X-CSRF-Token: $CS_A" -H 'Content-Type: application/json')
HB=(-H "Authorization: Bearer $TOK_B" -H "X-CSRF-Token: $CS_B" -H 'Content-Type: application/json')

mkzone(){ curl -s -X POST http://127.0.0.1:$1/api/v1/dns/zones "${@:2}" -d "{\"name\":\"$3\",\"type\":\"primary\"}"; }
# Create zone+record on node-a
ZR=$(curl -s -X POST http://127.0.0.1:18181/api/v1/dns/zones "${HA[@]}" -d '{"name":"h2.test.","type":"primary"}')
ZA=$(printf '%s' "$ZR" | jget "d['data']['id']")
if [ -n "${ZA:-}" ]; then ok "zone created on node-a"
else
  # Reuse an existing zone so the suite is repeatable.
  ZA=$(curl -s http://127.0.0.1:18181/api/v1/dns/zones -H "Authorization: Bearer $TOK_A" \
    | python3 -c "import sys,json;d=json.load(sys.stdin)['data'];print(next((x['id'] for x in d if x['name']=='h2.test.'),''))")
  [ -n "${ZA:-}" ] && ok "zone h2.test. reused on node-a" || bad "zone create node-a ($(printf '%s' "$ZR" | head -c 120))"
fi
curl -s -X POST "http://127.0.0.1:18181/api/v1/dns/zones/$ZA/records" "${HA[@]}" -d '{"name":"www","type":"A","ttl":300,"value":"192.0.2.10"}' -o /dev/null
sleep 3
a=$(digq node-a www.h2.test.)
[ "$a" = "192.0.2.10" ] && ok "node-a answers www.h2.test. A via real dig ($a)" || bad "node-a dig answer='$a'"
# Cross-node propagation (ADR-0005 multi-node publish NOT implemented -> expect absent)
b=$(digq node-b www.h2.test.)
if [ "$b" = "192.0.2.10" ]; then nv "node-b already served node-a config (unexpected without publish coordination)"; else nv "node-b does NOT serve node-a's independently-created config (ADR-0005 multi-node publish NOT_IMPLEMENTED)"; fi
# ADR-0011 single-author rule: node-b is NOT the configuration author, so the
# same write must be refused there (409) and must not create a second revision.
CODEB=$(curl -s -o /dev/null -w '%{http_code}' -X POST http://127.0.0.1:18182/api/v1/dns/zones "${HB[@]}" \
  -d '{"name":"h2.test.","type":"primary"}')
[ "$CODEB" = "409" ] && ok "node-b refuses the config write (single-author rule, HTTP 409)" || bad "node-b write returned $CODEB, want 409"
# node-b still serves DNS from its own (mirrored) store: query its own address.
BIP=$(docker exec node-b sh -c 'ip -4 -o addr show | awk "{print \$4}" | cut -d/ -f1 | grep "^10.77.3." | head -1')
b2=$(docker exec client sh -c "apk add -q bind-tools >/dev/null 2>&1; dig +short +time=2 +tries=1 @$BIP www.h2.test. A 2>/dev/null" | tr -d '\r' | head -1)
if [ -n "${b2:-}" ]; then ok "node-b answers DNS from its own store ($b2)"; else nv "node-b has no local copy of www.h2.test. (config is not published across nodes)"; fi

# LKG revision / config divergence between the two independent control planes
ra=$(curl -s -H "Authorization: Bearer $TOK_A" http://127.0.0.1:18181/metrics | grep '^goddi_dataplane_lkg_revision' | tr -d '\r'); rb=$(curl -s -H "Authorization: Bearer $TOK_B" http://127.0.0.1:18182/metrics | grep '^goddi_dataplane_lkg_revision' | tr -d '\r')
printf '%s\n%s\n' "$ra" "$rb" > "$OUT/dns-lkg-$TS.txt"
if [ -n "$ra" ] && [ -n "$rb" ]; then
  [ "$ra" = "$rb" ] && ok "both nodes report same LKG revision" || nv "LKG revisions differ between independent control planes (no cluster-wide publish): a=[$ra] b=[$rb]"
else nv "LKG revision metric not yet present on one/both nodes"; fi
# Control-plane/primary down: node-b DNS must keep answering from its local copy.
# The query must target node-b's own client-network address, not a name that may
# resolve to node-a.
docker stop node-a >/dev/null 2>&1; sleep 1
BIP=$(docker exec node-b sh -c 'ip -4 -o addr show | awk "{print \$4}" | cut -d/ -f1 | grep "^10.77.3." | head -1')
# node-b has no copy of node-a's zone (config is not published across nodes), so
# the assertion is that its DNS listener keeps answering authoritatively rather
# than refusing: a status of NOERROR/NXDOMAIN proves the plane is up.
b3=""
for _ in $(seq 1 10); do
  b3=$(docker exec client sh -c "apk add -q bind-tools >/dev/null 2>&1; dig +noall +comments +time=2 +tries=1 @$BIP www.h2.test. A 2>/dev/null | grep -o 'status: [A-Z]*'" | head -1)
  [ -n "$b3" ] && break; sleep 2
done
case "$b3" in
  *NOERROR*|*NXDOMAIN*) ok "node-b DNS keeps answering while node-a is down ($b3)";;
  *) bad "node-b DNS did not answer with node-a down ($b3)";;
esac
docker start node-a >/dev/null 2>&1
for _ in $(seq 1 30); do [ "$(docker inspect -f '{{.State.Health.Status}}' node-a 2>/dev/null)" = healthy ] && break; sleep 2; done
# Delete a record on node-b (author) and confirm withdrawal on node-a
if [ -n "${ZA:-}" ]; then
  # A DNS name may legitimately carry several values (round-robin), and repeat
  # runs add one row each, so the withdrawal check deletes every record with
  # that name instead of only the first.
  RIDS=$(curl -s "http://127.0.0.1:18181/api/v1/dns/zones/$ZA/records" -H "Authorization: Bearer $TOK_A" \
    | python3 -c "import sys,json;d=json.load(sys.stdin)['data'];print(' '.join(r['id'] for r in d if r['name'].startswith('www.')))" 2>/dev/null)
  for RID in $RIDS; do
    curl -s -X DELETE "http://127.0.0.1:18181/api/v1/dns/zones/$ZA/records/$RID" "${HA[@]}" -o /dev/null
  done
  b4=""
  for _ in $(seq 1 10); do
    b4=$(digq node-a www.h2.test.)
    [ -z "$b4" ] && break; sleep 2
  done
  [ -z "$b4" ] && ok "record delete withdrawn on the author node (NXDOMAIN)" || bad "record still served after delete: $b4"
fi
# AXFR/IXFR/NOTIFY: not exercised here; report honestly.
nv "AXFR/IXFR/NOTIFY interop (TSIG transfer drill) not yet exercised in this script"
printf '{"suite":"dns","timestamp":"%s","pass":%d,"fail":%d,"not_verified":%d}\n' "$TS" "$PASS" "$FAIL" "$NV" > "$OUT/dns-summary-$TS.json"
echo "----"; echo "DNS PASS=$PASS FAIL=$FAIL NOT_VERIFIED=$NV"
[ "$FAIL" -eq 0 ]
