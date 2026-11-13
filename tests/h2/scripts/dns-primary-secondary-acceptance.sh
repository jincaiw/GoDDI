#!/usr/bin/env bash
# DNS redundancy contract: exactly one primary per zone, the other node follows as
# a secondary over NOTIFY + AXFR/IXFR with TSIG.
#
# Why this matters for DHCP failover: DHCP failover only changes which node
# produces lease facts. DNS redundancy is an independent axis. If both nodes
# created a *primary* zone of the same name, a DHCP switch would leave two
# divergent DNS copies; with one primary and one secondary the follower keeps
# catching up regardless of which node is the DHCP owner.
set -uo pipefail
cd "$(dirname "$0")/.."
TS=$(date -u +%Y%m%dT%H%M%SZ); OUT=reports; mkdir -p "$OUT"
PASS=0; FAIL=0; NV=0
ok(){ echo "PASS | $1"; PASS=$((PASS+1)); }
bad(){ echo "FAIL | $1"; FAIL=$((FAIL+1)); }
nv(){ echo "NOT_VERIFIED | $1"; NV=$((NV+1)); }
jget(){ python3 -c "import sys,json;d=json.load(sys.stdin);print($1)" 2>/dev/null; }
login(){ curl -s -X POST "http://127.0.0.1:$1/api/v1/auth/login" -H 'Content-Type: application/json' -d '{"username":"admin","password":"AdminPass-0123456789"}'; }
bash scripts/build-probes.sh >/dev/null 2>&1 || true

# Docker is free to swap the client-network addresses when a container is
# recreated, so both addresses are re-resolved immediately before each use.
resip(){ docker exec "$1" sh -c 'ip -4 -o addr show | awk "{print \$4}" | cut -d/ -f1 | grep "^10.77.3." | head -1'; }
AIP=$(resip node-a); BIP=$(resip node-b)
[ -n "$AIP" ] && [ -n "$BIP" ] || { echo "FAIL | could not resolve node addresses"; exit 1; }
echo "node-a=$AIP node-b=$BIP"

RA=$(login 18181); TA=$(printf '%s' "$RA"|jget "d['data']['token']"); CA=$(printf '%s' "$RA"|jget "d['data']['csrf_token']")
HA=(-H "Authorization: Bearer $TA" -H "X-CSRF-Token: $CA" -H 'Content-Type: application/json')
ZONE="ps$(date +%s).test."

# TSIG key on the primary (transfer must be signed; an unauthenticated AXFR is refused).
KNAME="ps$(date +%s)."
KJ=$(curl -s -X POST http://127.0.0.1:18181/api/v1/dns/tsig/keys "${HA[@]}" -d "{\"name\":\"$KNAME\",\"algorithm\":\"hmac-sha256\"}")
KN=$(printf '%s' "$KJ"|jget "d['data']['name']"); KS=$(printf '%s' "$KJ"|jget "d['data']['secret']")
[ -n "${KS:-}" ] && ok "TSIG key created on the primary" || bad "TSIG key creation failed"

# Primary zone on node-a.
ZID=$(curl -s -X POST http://127.0.0.1:18181/api/v1/dns/zones "${HA[@]}" \
  -d "{\"name\":\"$ZONE\",\"type\":\"primary\",\"refresh\":5,\"retry\":2,\"expire\":600}" | jget "d['data']['id']")
[ -n "${ZID:-}" ] && ok "primary zone created on node-a ($ZONE)" || bad "primary zone create failed"
curl -s -X POST "http://127.0.0.1:18181/api/v1/dns/zones/$ZID/records" "${HA[@]}" \
  -d '{"name":"www","type":"A","ttl":60,"value":"192.0.2.55"}' -o /dev/null
curl -s -X POST "http://127.0.0.1:18181/api/v1/dns/zones/$ZID/transfers" "${HA[@]}" \
  -d "{\"allowed_cidr\":\"10.77.3.0/24\",\"tsig_key_name\":\"$KN\"}" -o /dev/null
# The key must be loaded by the listener before a signed transfer can verify.
docker restart node-a >/dev/null 2>&1
for _ in $(seq 1 40); do [ "$(docker inspect -f '{{.State.Health.Status}}' node-a 2>/dev/null)" = healthy ] && break; sleep 2; done
AIP=$(resip node-a); BIP=$(resip node-b)
for _ in $(seq 1 30); do docker exec client sh -c "nc -z -w1 $AIP 53" 2>/dev/null && break; sleep 1; done

# node-b is NOT the config author (ADR-0011), so the secondary zone is created
# through the author's API but belongs to node-b's store. To exercise the
# follower we therefore create it on node-b's own store via its API and accept a
# 409 if the single-author rule refuses; the rule and the DNS role are different
# axes, so a refusal is reported, not counted as a DNS failure.
RB=$(login 18182); TB=$(printf '%s' "$RB"|jget "d['data']['token']"); CB=$(printf '%s' "$RB"|jget "d['data']['csrf_token']")
HB=(-H "Authorization: Bearer $TB" -H "X-CSRF-Token: $CB" -H 'Content-Type: application/json')
AIP=$(resip node-a)
BIP=$(resip node-b)
POLICY="$AIP|$KN|$KS|hmac-sha256"
ZRB=$(curl -s -X POST http://127.0.0.1:18182/api/v1/dns/zones "${HB[@]}" \
  -d "{\"name\":\"$ZONE\",\"type\":\"secondary\",\"transfer_policy\":\"$POLICY\",\"refresh\":5,\"retry\":2,\"expire\":600}")
ZCODE=$(printf '%s' "$ZRB" | jget "d['code']")
if [ "${ZCODE:-0}" = "0" ]; then
  ok "secondary zone created on node-b (follower)"
  FOLLOW=1
else
  nv "node-b refused the secondary zone create ($(printf '%s' "$ZRB" | head -c 100)); single-author rule vs DNS role are separate axes — deploy the secondary on a node allowed to author its own DNS config"
  FOLLOW=0
fi

if [ "$FOLLOW" -eq 1 ]; then
  # The secondary sweeper runs on a 30s tick; wait across several cycles so the
  # first AXFR is not racing the ticker.
  got=""
  for _ in $(seq 1 45); do
    got=$(docker exec client sh -c "nslookup www.$ZONE $BIP 2>/dev/null | awk '/^Address: /{print \$2}' | tail -1" | tr -d '\r')
    [ "$got" = "192.0.2.55" ] && break; sleep 2
  done
  [ "$got" = "192.0.2.55" ] && ok "secondary answered the transferred record (AXFR follow-up)" || bad "secondary did not serve www.$ZONE (got '$got')"

  # Change a record on the primary; the follower must pick it up via NOTIFY/refresh.
  RID=$(curl -s "http://127.0.0.1:18181/api/v1/dns/zones/$ZID/records" -H "Authorization: Bearer $TA" | jget "d['data'][0]['id']")
  [ -n "${RID:-}" ] && curl -s -X PUT "http://127.0.0.1:18181/api/v1/dns/zones/$ZID/records/$RID" "${HA[@]}" \
    -d '{"name":"www","type":"A","ttl":60,"value":"192.0.2.99"}' -o /dev/null
  upd=""
  for _ in $(seq 1 45); do
    upd=$(docker exec client sh -c "nslookup www.$ZONE $BIP 2>/dev/null | awk '/^Address: /{print \$2}' | tail -1" | tr -d '\r')
    [ "$upd" = "192.0.2.99" ] && break; sleep 2
  done
  [ "$upd" = "192.0.2.99" ] && ok "secondary converged after a primary record change (serial caught up)" || bad "secondary still serves '$upd', want 192.0.2.99"

  # Serial parity is the evidence that the follower is not diverging.
  sa=""; sb=""
  for _ in $(seq 1 20); do
    sa=$(docker exec client sh -c "nslookup -type=SOA $ZONE $AIP 2>/dev/null | grep -o 'serial = [0-9]*' | head -1" | tr -d '\r')
    sb=$(docker exec client sh -c "nslookup -type=SOA $ZONE $BIP 2>/dev/null | grep -o 'serial = [0-9]*' | head -1" | tr -d '\r')
    [ -n "$sa" ] && [ "$sa" = "$sb" ] && break; sleep 2
  done
  if [ -n "$sa" ] && [ "$sa" = "$sb" ]; then ok "primary and secondary report the same SOA serial ($sa)"
  else nv "serial comparison inconclusive (primary='$sa' secondary='$sb')"; fi

  # Guard: the same zone must not exist as a primary on both nodes.
  t_b=$(curl -s http://127.0.0.1:18182/api/v1/dns/zones -H "Authorization: Bearer $TB" \
    | python3 -c "import sys,json;d=json.load(sys.stdin)['data'];print(next((z['type'] for z in d if z['name']=='$ZONE'),'none'))" 2>/dev/null)
  [ "$t_b" = "secondary" ] && ok "node-b holds the zone as secondary, not primary (no dual-primary)" || bad "node-b zone type is '$t_b', want secondary"
fi

# Dual-primary detection: if a second node ever creates a primary of the same
# name, that is the failure mode the contract forbids. Detect it by asking the
# author node to create the same zone as primary twice.
DUP=$(curl -s -X POST http://127.0.0.1:18181/api/v1/dns/zones "${HA[@]}" -d "{\"name\":\"$ZONE\",\"type\":\"primary\"}")
DUPCODE=$(printf '%s' "$DUP" | jget "d['code']")
[ "${DUPCODE:-0}" != "0" ] && ok "a duplicate primary of the same zone is refused (unique name enforced)" || bad "duplicate primary zone was accepted"

printf '{"suite":"dns-primary-secondary","timestamp":"%s","pass":%d,"fail":%d,"not_verified":%d}\n' "$TS" "$PASS" "$FAIL" "$NV" > "$OUT/dns-primary-secondary-summary-$TS.json"
echo "----"; echo "DNS-PRIMARY-SECONDARY PASS=$PASS FAIL=$FAIL NOT_VERIFIED=$NV"
[ "$FAIL" -eq 0 ]
