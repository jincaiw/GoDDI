#!/usr/bin/env bash
# Core software-level consistency checks against a running H2 pair.
# Emits machine-readable JSON to reports/ and PASS/FAIL lines.
set -uo pipefail
cd "$(dirname "$0")/.."
TS=$(date -u +%Y%m%dT%H%M%SZ); OUT="reports"; mkdir -p "$OUT"
PASS=0; FAIL=0; NV=0
ok(){ echo "PASS | $1"; PASS=$((PASS+1)); }
bad(){ echo "FAIL | $1"; FAIL=$((FAIL+1)); }
nv(){ echo "NOT_VERIFIED | $1"; NV=$((NV+1)); }
mg(){ docker exec "$1" wget -qO- "http://localhost:6080$2" 2>/dev/null; }
check(){ n="$1";
  if docker exec "$n" sh -c 'wget --spider -q http://localhost:6080/health' 2>/dev/null; then ok "$n health"; else bad "$n health"; fi
  h=$(docker inspect -f '{{.State.Health.Status}}' "$n" 2>/dev/null); [ "$h" = healthy ] && ok "$n container healthy" || bad "$n container healthy=$h"
}
check node-a; check node-b
# Exactly-one-owner signal via the CLI (no management auth needed).
ha_status(){ docker exec "$1" goddi ha status -c /etc/goddi/config.yaml 2>/dev/null; }
sa=$(ha_status node-a); sb=$(ha_status node-b)
printf '%s\n' "$sa" > "$OUT/ha-node-a-$TS.txt"; printf '%s\n' "$sb" > "$OUT/ha-node-b-$TS.txt"
[ -n "$sa" ] && ok "node-a HA status reachable" || bad "node-a HA status unreachable"
[ -n "$sb" ] && ok "node-b HA status reachable" || bad "node-b HA status unreachable"
# Exactly one owner: node-a role-in-force primary, node-b role-in-force standby.
a_primary=$(printf '%s' "$sa" | grep -c "role in force:    primary")
b_primary=$(printf '%s' "$sb" | grep -c "role in force:    primary")
if [ "$a_primary" -eq 1 ] && [ "$b_primary" -eq 0 ]; then ok "exactly one owner (node-a primary, node-b standby)"; else bad "owner uniqueness (a_primary=$a_primary b_primary=$b_primary)"; fi
# mTLS peer channel in use (startup log marker). Capture first, then match via
# case, so `set -o pipefail` + `grep -q` SIGPIPE cannot flip a real marker to
# missing.
la=$(docker logs node-a 2>&1); lb=$(docker logs node-b 2>&1)
case "$la" in *"mutual TLS"*) ok "node-a replication uses mutual TLS";; *) bad "node-a mTLS marker missing";; esac
case "$lb" in *"mutual TLS"*) ok "node-b replication uses mutual TLS";; *) bad "node-b mTLS marker missing";; esac
case "$la" in *"data plane LKG"*) ok "node-a LKG stamp logged";; *) nv "node-a LKG stamp not yet logged";; esac
case "$lb" in *"data plane LKG"*) ok "node-b LKG stamp logged";; *) nv "node-b LKG stamp not yet logged";; esac
# Physical/hardware items are out of scope for Docker H2.
nv "real power-loss durability (Level 2 physical)"
nv "SSD/eMMC cache reliability (Level 2 physical)"
printf '{"timestamp":"%s","pass":%d,"fail":%d,"not_verified":%d}\n' "$TS" "$PASS" "$FAIL" "$NV" > "$OUT/summary-$TS.json"
echo "----"; echo "PASS=$PASS FAIL=$FAIL NOT_VERIFIED=$NV (report $OUT/summary-$TS.json)"
[ "$FAIL" -eq 0 ]
