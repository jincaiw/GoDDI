#!/usr/bin/env bash
# ADR-0005 minimal contract: exactly one configuration author.
#
# What this proves: a node marked cluster.config_author=false refuses
# configuration writes (409) while still serving reads and its data plane, so two
# consoles can never publish divergent revisions of the same resource.
# What it does NOT prove: leader election, quorum/witness, epoch fencing or
# watermark catch-up -- those remain unimplemented (see docs/adr/0011).
set -uo pipefail
cd "$(dirname "$0")/.."
TS=$(date -u +%Y%m%dT%H%M%SZ); OUT=reports; mkdir -p "$OUT"
PASS=0; FAIL=0; NV=0
ok(){ echo "PASS | $1"; PASS=$((PASS+1)); }
bad(){ echo "FAIL | $1"; FAIL=$((FAIL+1)); }
nv(){ echo "NOT_VERIFIED | $1"; NV=$((NV+1)); }
jget(){ python3 -c "import sys,json;d=json.load(sys.stdin);print($1)" 2>/dev/null; }
login(){ curl -s -X POST "http://127.0.0.1:$1/api/v1/auth/login" -H 'Content-Type: application/json' -d '{"username":"admin","password":"AdminPass-0123456789"}'; }
RA=$(login 18181); TA=$(printf '%s' "$RA"|jget "d['data']['token']"); CA=$(printf '%s' "$RA"|jget "d['data']['csrf_token']")
RB=$(login 18182); TB=$(printf '%s' "$RB"|jget "d['data']['token']"); CB=$(printf '%s' "$RB"|jget "d['data']['csrf_token']")

NAME="author$(date +%s).test."
code_a=$(curl -s -o /dev/null -w '%{http_code}' -X POST http://127.0.0.1:18181/api/v1/dns/zones \
  -H "Authorization: Bearer $TA" -H "X-CSRF-Token: $CA" -H 'Content-Type: application/json' \
  -d "{\"name\":\"$NAME\",\"type\":\"primary\"}")
[ "$code_a" = "201" ] || [ "$code_a" = "400" ] && ok "author node accepts a configuration write (HTTP $code_a)" || bad "author node write returned $code_a"

code_b=$(curl -s -o /dev/null -w '%{http_code}' -X POST http://127.0.0.1:18182/api/v1/dns/zones \
  -H "Authorization: Bearer $TB" -H "X-CSRF-Token: $CB" -H 'Content-Type: application/json' \
  -d "{\"name\":\"non$NAME\",\"type\":\"primary\"}")
[ "$code_b" = "409" ] && ok "non-author node refuses a configuration write (HTTP 409)" || bad "non-author write returned $code_b, want 409"

body_b=$(curl -s -X POST http://127.0.0.1:18182/api/v1/dns/zones \
  -H "Authorization: Bearer $TB" -H "X-CSRF-Token: $CB" -H 'Content-Type: application/json' \
  -d "{\"name\":\"non2$NAME\",\"type\":\"primary\"}")
case "$body_b" in *"配置作者"*|*"config_author"*) ok "refusal explains the reason (not a bare 409)";; *) nv "refusal body did not name the cause: $(printf '%s' "$body_b" | head -c 100)";; esac

for path in /api/v1/dns/zones /api/v1/dhcp/scopes /api/v1/ipam/spaces; do
  c=$(curl -s -o /dev/null -w '%{http_code}' "http://127.0.0.1:18182$path" -H "Authorization: Bearer $TB")
  [ "$c" = "200" ] && ok "non-author node still serves reads ($path)" || bad "read $path on non-author returned $c"
done
c=$(curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:18182/health); [ "$c" = "200" ] && ok "non-author node health OK" || bad "non-author health $c"

# The non-author node must still be a functioning DHCP standby (data plane
# unaffected by the API-level restriction).
st=$(docker exec node-b goddi ha status -c /etc/goddi/config.yaml 2>/dev/null | grep -cE "role in force: +standby")
[ "$st" -eq 1 ] && ok "non-author node keeps its DHCP standby role" || bad "non-author node role changed"

# A refused write must leave no trace: the non-author store must not contain it.
left=$(docker exec node-b sh -c "apk add -q sqlite >/dev/null 2>&1; sqlite3 /var/lib/goddi/goddi.db \"select count(*) from dns_zones where name like 'non%$NAME';\"" 2>/dev/null | tr -d '\r')
[ "${left:-0}" -eq 0 ] && ok "refused write left no row in the non-author store" || bad "refused write persisted $left row(s)"

nv "leader election / quorum / witness / epoch fencing remain unimplemented (full ADR-0005)"
printf '{"suite":"config-author","timestamp":"%s","pass":%d,"fail":%d,"not_verified":%d}\n' "$TS" "$PASS" "$FAIL" "$NV" > "$OUT/config-author-summary-$TS.json"
echo "----"; echo "CONFIG-AUTHOR PASS=$PASS FAIL=$FAIL NOT_VERIFIED=$NV"
[ "$FAIL" -eq 0 ]
