#!/usr/bin/env bash
# Dual-node IPAM lifecycle archive matrix on the running H2 pair.
# Parent lifecycle is configuration and replicates; local address/history rows
# are not configuration and must not. Docker H2 evidence, not production proof.
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
sqla(){ printf '%s\n' "$1" | docker exec -i node-a sqlite3 -noheader /var/lib/goddi/goddi.db 2>/dev/null | tr -d '\r'; }
sqlb(){ printf '%s\n' "$1" | docker exec -i node-b sqlite3 -noheader /var/lib/goddi/goddi.db 2>/dev/null | tr -d '\r'; }

echo "### pair with ADR-0013 config replication"
docker compose -f docker-compose.yml -f docker-compose.configrepl.yml down -v --remove-orphans >/dev/null 2>&1
docker compose -f docker-compose.yml -f docker-compose.configrepl.yml up -d >/dev/null 2>&1
if waithealth node-a && waithealth node-b; then ok "pair healthy with config replication"; else bad "pair did not come up"; exit 1; fi
RA=$(login 18181); TA=$(printf '%s' "$RA"|jget "d['data']['token']"); CA=$(printf '%s' "$RA"|jget "d['data']['csrf_token']")
HA=(-H "Authorization: Bearer $TA" -H "X-CSRF-Token: $CA" -H 'Content-Type: application/json')
RB=$(login 18182); TB=$(printf '%s' "$RB"|jget "d['data']['token']")

echo "### author parent and local child data on node-a"
NAME="h2-archive-$TS"
SP=$(curl -s -X POST http://127.0.0.1:18181/api/v1/ipam/spaces "${HA[@]}" -d "{\"name\":\"$NAME\",\"description\":\"archive matrix\"}" | jget "d['data']['id']")
[ -n "${SP:-}" ] && ok "space authored on node-a ($SP)" || { bad "space create failed"; exit 1; }
SN=$(curl -s -X POST http://127.0.0.1:18181/api/v1/ipam/subnets "${HA[@]}" -d "{\"space_id\":\"$SP\",\"name\":\"h2-archive-sub-$TS\",\"cidr\":\"10.77.3.0/24\",\"location\":\"h2-archive\"}" | jget "d['data']['id']")
[ -n "${SN:-}" ] && ok "subnet authored on node-a ($SN)" || { bad "subnet create failed"; exit 1; }
ALLOC=$(curl -s -X POST http://127.0.0.1:18181/api/v1/ipam/addresses/allocate "${HA[@]}" -d "{\"subnet_id\":\"$SN\",\"ip_address\":\"10.77.3.199\",\"status\":\"used\",\"owner\":\"h2-archive\",\"reason\":\"archive matrix\"}" | jget "d['data']['id']")
[ -n "${ALLOC:-}" ] && ok "address intent written on node-a ($ALLOC)" || bad "address allocate failed"
ADDR_A=$(sqla "select count(*) from ipam_addresses where subnet_id='$SN';")
HIST_A=$(sqla "select count(*) from ipam_history where subnet_id='$SN';")
OBS_A=$(sqla "select count(*) from ipam_address_observations o join ipam_addresses a on a.id=o.address_id where a.subnet_id='$SN';")
[ "${ADDR_A:-0}" -ge 1 ] && ok "node-a has local address rows ($ADDR_A)" || bad "node-a address row missing"
[ "${HIST_A:-0}" -ge 1 ] && ok "node-a has local history rows ($HIST_A)" || bad "node-a history row missing"
[ "${OBS_A:-0}" -ge 1 ] && ok "node-a has split observation rows ($OBS_A)" || bad "node-a split observation row missing"

echo "### replication boundary before retirement"
sleep 6
SP_B=$(sqlb "select lifecycle_state from ipam_spaces where id='$SP';")
SN_B=$(sqlb "select lifecycle_state from ipam_subnets where id='$SN';")
[ "$SP_B" = "active" ] && ok "active space replicated to node-b" || bad "space missing on node-b (got '$SP_B')"
[ "$SN_B" = "active" ] && ok "active subnet replicated to node-b" || bad "subnet missing on node-b (got '$SN_B')"
ADDR_B=$(sqlb "select count(*) from ipam_addresses where subnet_id='$SN';")
HIST_B=$(sqlb "select count(*) from ipam_history where subnet_id='$SN';")
OBS_B=$(sqlb "select count(*) from ipam_address_observations o join ipam_addresses a on a.id=o.address_id where a.subnet_id='$SN';")
[ "${ADDR_B:-0}" -eq 0 ] && ok "address intent not replicated to node-b" || bad "node-b copied $ADDR_B local address rows"
[ "${HIST_B:-0}" -eq 0 ] && ok "history not replicated to node-b" || bad "node-b copied $HIST_B local history rows"
[ "${OBS_B:-0}" -eq 0 ] && ok "split observations not replicated to node-b" || bad "node-b copied $OBS_B split observation rows"

echo "### retire parent and verify tombstone replication"
curl -s -X DELETE "http://127.0.0.1:18181/api/v1/ipam/subnets/$SN?reason=archive-matrix" "${HA[@]}" -o /dev/null
curl -s -X DELETE "http://127.0.0.1:18181/api/v1/ipam/spaces/$SP?reason=archive-matrix" "${HA[@]}" -o /dev/null
replicated=no
for _ in $(seq 1 20); do
  SP_B=$(sqlb "select lifecycle_state from ipam_spaces where id='$SP';")
  SN_B=$(sqlb "select lifecycle_state from ipam_subnets where id='$SN';")
  [ "$SP_B" = "retired" ] && [ "$SN_B" = "retired" ] && { replicated=yes; break; }
  sleep 1
done
[ "$replicated" = "yes" ] && ok "retired parent and child replicated to node-b" || bad "retirement did not reach node-b (space=$SP_B subnet=$SN_B)"
ADDR_B2=$(sqlb "select count(*) from ipam_addresses where subnet_id='$SN';")
HIST_B2=$(sqlb "select count(*) from ipam_history where subnet_id='$SN';")
[ "${ADDR_B2:-0}" -eq 0 ] && [ "${HIST_B2:-0}" -eq 0 ] && ok "local children stayed absent on node-b after retirement" || bad "node-b gained local children after retirement (a=$ADDR_B2 h=$HIST_B2)"
ADDR_A2=$(sqla "select count(*) from ipam_addresses where subnet_id='$SN';")
HIST_A2=$(sqla "select count(*) from ipam_history where subnet_id='$SN';")
[ "${ADDR_A2:-${ADDR_A:-0}}" -ge 1 ] && [ "${HIST_A2:-${HIST_A:-0}}" -ge 1 ] && ok "author kept local address/history through retirement" || bad "author lost local children (a=$ADDR_A2 h=$HIST_A2)"

echo "### restore propagates; purge does not resurrect"
curl -s -X POST "http://127.0.0.1:18181/api/v1/ipam/spaces/$SP/restore" "${HA[@]}" -d '{}' -o /dev/null
curl -s -X POST "http://127.0.0.1:18181/api/v1/ipam/subnets/$SN/restore" "${HA[@]}" -d '{}' -o /dev/null
active=no
for _ in $(seq 1 20); do
  SP_B=$(sqlb "select lifecycle_state from ipam_spaces where id='$SP';")
  SN_B=$(sqlb "select lifecycle_state from ipam_subnets where id='$SN';")
  [ "$SP_B" = "active" ] && [ "$SN_B" = "active" ] && { active=yes; break; }
  sleep 1
done
[ "$active" = "yes" ] && ok "restore propagated to node-b" || bad "restore did not reach node-b (space=$SP_B subnet=$SN_B)"
EP=$(curl -s -X POST http://127.0.0.1:18181/api/v1/ipam/spaces "${HA[@]}" -d "{\"name\":\"h2-purge-$TS\"}" | jget "d['data']['id']")
curl -s -X DELETE "http://127.0.0.1:18181/api/v1/ipam/spaces/$EP?reason=archive-matrix" "${HA[@]}" -o /dev/null
sleep 6
[ "$(sqlb "select lifecycle_state from ipam_spaces where id='$EP';")" = "retired" ] && ok "empty tombstone replicated before purge" || bad "empty tombstone missing on node-b"
curl -s -X POST "http://127.0.0.1:18181/api/v1/ipam/spaces/$EP/purge" "${HA[@]}" -d "{\"confirmation\":\"$EP\"}" -o /dev/null
purged=no
for _ in $(seq 1 20); do
  [ "$(sqlb "select count(*) from ipam_spaces where id='$EP';")" = "0" ] && { purged=yes; break; }
  sleep 1
done
[ "$purged" = "yes" ] && ok "purge removed the parent on node-b (no resurrection)" || bad "purged parent still present on node-b"

echo "### restore standard pair"
docker compose -f docker-compose.yml -f docker-compose.configrepl.yml down -v --remove-orphans >/dev/null 2>&1
docker compose -f docker-compose.yml up -d >/dev/null 2>&1
waithealth node-a && waithealth node-b && ok "restored the standard pair" || bad "failed to restore standard pair"

printf '{"suite":"ipam-lifecycle-replication","timestamp":"%s","pass":%d,"fail":%d,"not_verified":%d}\n' "$TS" "$PASS" "$FAIL" "$NV" > "$OUT/ipam-lifecycle-replication-summary-$TS.json"
echo "----"; echo "IPAM-LIFECYCLE-REPLICATION PASS=$PASS FAIL=$FAIL NOT_VERIFIED=$NV"
[ "$FAIL" -eq 0 ]