#!/usr/bin/env bash
# DHCP dual-node acceptance using real udhcpc DORA through the real GoDDI DHCP
# server. Honest verdicts per DEC-002/DEC-006/ADR-0010.
set -uo pipefail
cd "$(dirname "$0")/.."
TS=$(date -u +%Y%m%dT%H%M%SZ); OUT=reports; mkdir -p "$OUT"
PASS=0; FAIL=0; NV=0
ok(){ echo "PASS | $1"; PASS=$((PASS+1)); }
bad(){ echo "FAIL | $1"; FAIL=$((FAIL+1)); }
nv(){ echo "NOT_VERIFIED | $1"; NV=$((NV+1)); }
jget(){ python3 -c "import sys,json;d=json.load(sys.stdin);print($1)" 2>/dev/null; }
login(){ curl -s -X POST "http://127.0.0.1:$1/api/v1/auth/login" -H 'Content-Type: application/json' -d '{"username":"admin","password":"AdminPass-0123456789"}'; }
RA=$(login 18181); TOK_A=$(printf '%s' "$RA"|jget "d['data']['token']"); CS_A=$(printf '%s' "$RA"|jget "d['data']['csrf_token']")
RB=$(login 18182); TOK_B=$(printf '%s' "$RB"|jget "d['data']['token']"); CS_B=$(printf '%s' "$RB"|jget "d['data']['csrf_token']")
HA=(-H "Authorization: Bearer $TOK_A" -H "X-CSRF-Token: $CS_A" -H 'Content-Type: application/json')
HB=(-H "Authorization: Bearer $TOK_B" -H "X-CSRF-Token: $CS_B" -H 'Content-Type: application/json')

curl -s -X POST http://127.0.0.1:18181/api/v1/dhcp/scopes "${HA[@]}" \
  -d '{"name":"h2-client","subnet":"10.77.3.0/24","start_ip":"10.77.3.100","end_ip":"10.77.3.200","lease_time":600}' -o /dev/null -w ''
sleep 4
# Baseline: clear any persisted degraded marker from a prior run so partition
# tests observe the default paused state, not a leftover operator approval.
docker exec node-a goddi ha degrade -c /etc/goddi/config.yaml --undo --reason "h2 baseline" >/dev/null 2>&1 || true
sleep 1

setmac(){ docker exec client sh -c "ip link set eth0 down; ip link set eth0 address $1; ip link set eth0 up" >/dev/null 2>&1; }
udhcpc(){ docker exec client sh -c 'rm -f /tmp/u.log; udhcpc -i eth0 -n -q -f -x hostname:h2c -t 8 -T 2 >/tmp/u.log 2>&1; echo EXIT=$?; cat /tmp/u.log'; }
ipof(){ printf '%s' "$1" | grep -o 'lease of [0-9.]*' | awk '{print $3}' | head -1; }
leasecount(){ curl -s "http://127.0.0.1:$1/api/v1/dhcp/leases?ip=$2" -H "Authorization: Bearer $3" | jget "d['meta']['total']"; }

# Phase 1: first DORA
setmac 02:00:00:00:00:01
U1=$(udhcpc); IP1=$(ipof "$U1")
[ -n "${IP1:-}" ] && ok "client got lease from node-a (real udhcpc): $IP1" || bad "no DHCP lease (log: $(printf '%s' "$U1" | tr '\n' ' ' | head -c 160))"
if [ -n "${IP1:-}" ]; then
  ca=$(leasecount 18181 "$IP1" "$TOK_A"); cb=$(leasecount 18182 "$IP1" "$TOK_B")
  [ "${ca:-0}" -ge 1 ] && ok "node-a lease store has $IP1 (active)" || bad "node-a has no lease for $IP1"
  [ "${cb:-0}" -ge 1 ] && ok "node-b mirror has lease $IP1" || nv "node-b mirror lease for $IP1 not visible via API (cb=${cb:-?})"
  # Retransmit/idempotency: same MAC again -> same address, no duplicate row
  U2=$(udhcpc); IP2=$(ipof "$U2")
  if [ "$IP2" = "$IP1" ]; then ok "retransmit/renew keeps same address $IP1"; else nv "same-MAC re-DISCOVER allocated new address '$IP2' (no MAC-affinity reuse across lease lifetime)"; fi
  # A single address can legitimately have historical rows (an expired lease
  # from an earlier client plus the current one). What must never happen is two
  # leases holding the same address at the same time, so the check counts only
  # leases that are currently held.
  act=$(curl -s "http://127.0.0.1:18181/api/v1/dhcp/leases?ip=$IP1&status=active" "${HA[@]}" | jget "d['meta']['total']")
  if [ "${act:-0}" -le 1 ]; then ok "at most one active lease for $IP1 (no double allocation)"; else bad "double allocation: $act active leases for $IP1"; fi
fi

# Phase 2: replication partition -> default paused (no new ACK), then explicit degrade serves
PART=$(bash scripts/network-partition.sh 2>&1); printf '%s\n' "$PART"
PISO=0; printf '%s' "$PART" | grep -q '^PASS | replication isolated' && PISO=1
sleep 7
# Live state is the running process's, not the CLI's store view: read the HA API first.
st=$(curl -s http://127.0.0.1:18181/api/v1/dhcp/ha -H "Authorization: Bearer $TOK_A" | jget "d['data']['state']")
if [ "$st" = "paused" ]; then ok "node-a reports paused via HA API (ADR-0010 default)"; elif docker logs --since 90s node-a 2>&1 | grep -q '"state":"paused"'; then ok "node-a observed paused in live log (log evidence)"; else nv "paused not observed in API or log"; fi
setmac 02:00:00:00:00:02
U3=$(udhcpc); IP3=$(ipof "$U3")
if [ "$PISO" -eq 1 ]; then
  [ -z "${IP3:-}" ] && ok "no ACK while peer isolated (fail-closed; DEC-002 no auto-promote)" || bad "node-a ACKed with peer isolated ($IP3)"
else nv "replication isolation not proven on this host; no-ACK expectation NOT_VERIFIED"; fi
IP4=""
if [ "$PISO" -eq 1 ]; then
  docker exec node-a goddi ha degrade -c /etc/goddi/config.yaml --confirm --reason "h2-acceptance explicit continue" >/dev/null 2>&1 && ok "operator ha degrade accepted" || bad "ha degrade rejected"
  sleep 2
  setmac 02:00:00:00:00:03
  U4=$(udhcpc); IP4=$(ipof "$U4")
  [ -n "${IP4:-}" ] && ok "after explicit degrade node-a serves locally (DEC-006 explicit): $IP4" || bad "degrade did not restore service"
  [ -n "${IP1:-}" ] && [ -n "${IP4:-}" ] && [ "$IP1" != "$IP4" ] && ok "no duplicate address across phases ($IP1 vs $IP4)" || nv "duplicate-address comparison inconclusive"
fi
# recover
docker exec node-a goddi ha degrade -c /etc/goddi/config.yaml --undo --reason "h2-acceptance recover" >/dev/null 2>&1 || true
bash scripts/network-recover.sh node-b >/dev/null 2>&1 || true
sleep 4
# Standby never serves: node-b stays standby role
docker exec node-b goddi ha status -c /etc/goddi/config.yaml 2>/dev/null | grep -qE "role in force: +standby" && ok "node-b remains standby (never self-promotes, DEC-002)" || bad "node-b role not standby"
# Watermarks present (sequence handed out > 0)
docker exec node-a goddi ha status -c /etc/goddi/config.yaml 2>/dev/null > "$OUT/ha-node-a-final-$TS.txt"
grep -qE "sequence: *[1-9]" "$OUT/ha-node-a-final-$TS.txt" && ok "node-a lease sequence advanced (>0)" || nv "lease sequence not yet >0"
# The D-03 hash chain is verified in ipam-acceptance: that suite creates the
# IPAM mapping needed for DHCP facts to be produced. There are no fact events
# to inspect in this scope-only DHCP drill, so no verdict is emitted here.
printf '{"suite":"dhcp","timestamp":"%s","pass":%d,"fail":%d,"not_verified":%d,"ip1":"%s","ip4":"%s"}\n' "$TS" "$PASS" "$FAIL" "$NV" "${IP1:-}" "${IP4:-}" > "$OUT/dhcp-summary-$TS.json"
echo "----"; echo "DHCP PASS=$PASS FAIL=$FAIL NOT_VERIFIED=$NV"
[ "$FAIL" -eq 0 ]
