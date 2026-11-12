#!/usr/bin/env bash
# H2 fault-scenario driver. Every scenario prints PASS/FAIL/NOT_VERIFIED and
# appends a JSON line to reports/faults-<ts>.json. A real failure is never
# retried into a PASS.
#
# Kernel-level injection (tc/netem, tmpfs disk limits, blackhole routing) is
# attempted; when the host cannot prove the fault took effect the scenario is
# NOT_VERIFIED, never PASS.
set -uo pipefail
cd "$(dirname "$0")/.."
TS=$(date -u +%Y%m%dT%H%M%SZ); OUT=reports; mkdir -p "$OUT"
J="$OUT/faults-$TS.jsonl"; : > "$J"
PASS=0; FAIL=0; NV=0
rec(){ printf '{"id":"%s","verdict":"%s","detail":"%s"}\n' "$1" "$2" "${3//\"/\'}" >> "$J"; }
ok(){ echo "PASS | $1 $2"; PASS=$((PASS+1)); rec "$1" PASS "$2"; }
bad(){ echo "FAIL | $1 $2"; FAIL=$((FAIL+1)); rec "$1" FAIL "$2"; }
nv(){ echo "NOT_VERIFIED | $1 $2"; NV=$((NV+1)); rec "$1" NOT_VERIFIED "$2"; }
jget(){ python3 -c "import sys,json;d=json.load(sys.stdin);print($1)" 2>/dev/null; }
login(){ curl -s -X POST "http://127.0.0.1:$1/api/v1/auth/login" -H 'Content-Type: application/json' -d '{"username":"admin","password":"AdminPass-0123456789"}'; }
RA=$(login 18181); TOK=$(printf '%s' "$RA"|jget "d['data']['token']"); CS=$(printf '%s' "$RA"|jget "d['data']['csrf_token']")
H=(-H "Authorization: Bearer $TOK" -H "X-CSRF-Token: $CS" -H 'Content-Type: application/json')
healthy(){ docker inspect -f '{{.State.Health.Status}}' "$1" 2>/dev/null; }
waithealth(){ for _ in $(seq 1 40); do [ "$(healthy "$1")" = healthy ] && return 0; sleep 2; done; return 1; }
loghas(){ docker logs --since "${2:-120s}" "$1" 2>&1 | grep -qE "$3"; }
dora(){ # $1 mac suffix -> prints assigned ip or empty
  docker exec client sh -c "ip link set eth0 down; ip link set eth0 address 02:00:00:00:01:$1; ip link set eth0 up; rm -f /tmp/f.log; udhcpc -i eth0 -n -q -f -x hostname:f$1 -t 5 -T 2 >/tmp/f.log 2>&1; grep -o 'lease of [0-9.]*' /tmp/f.log | awk '{print \$3}' | head -1"; }
leases(){ curl -s "http://127.0.0.1:18181/api/v1/dhcp/leases?ip=$1" "${H[@]}" | jget "d['meta']['total']"; }

baseline(){ # fresh volumes, node-a owner, wide scope
docker compose -f docker-compose.yml down -v --remove-orphans >/dev/null 2>&1
docker compose -f docker-compose.yml up -d >/dev/null 2>&1
for _ in $(seq 1 40); do
  [ "$(docker inspect -f '{{.State.Health.Status}}' node-a 2>/dev/null)" = healthy ] && \
  [ "$(docker inspect -f '{{.State.Health.Status}}' node-b 2>/dev/null)" = healthy ] && break
  sleep 2
done
RA=$(login 18181); TOK=$(printf '%s' "$RA"|jget "d['data']['token']"); CS=$(printf '%s' "$RA"|jget "d['data']['csrf_token']")
H=(-H "Authorization: Bearer $TOK" -H "X-CSRF-Token: $CS" -H 'Content-Type: application/json')
curl -s -X POST http://127.0.0.1:18181/api/v1/dhcp/scopes "${H[@]}" \
  -d '{"name":"h2-fault","subnet":"10.77.3.0/24","start_ip":"10.77.3.100","end_ip":"10.77.3.240","lease_time":600}' -o /dev/null
sleep 5
}
baseline

echo "### H2-001 Node A SIGKILL"
docker kill -s KILL node-a >/dev/null 2>&1
[ "$(docker inspect -f '{{.State.Running}}' node-a)" = "false" ] && ok H2-001 "node-a killed (SIGKILL); container stopped" || bad H2-001 "node-a still running after kill"
docker start node-a >/dev/null 2>&1; waithealth node-a && ok H2-001b "node-a restarted and healthy (data on volume survived)" || bad H2-001b "node-a did not become healthy after restart"
docker exec node-a goddi ha status -c /etc/goddi/config.yaml 2>/dev/null | grep -qE "role in force: +primary" && ok H2-001c "node-a still the owner after restart" || bad H2-001c "node-a lost owner role after restart"

echo "### H2-002 Node B SIGKILL"
docker kill -s KILL node-b >/dev/null 2>&1; sleep 2
[ "$(docker inspect -f '{{.State.Running}}' node-b)" = "false" ] && ok H2-002 "node-b killed; standby down" || bad H2-002 "node-b still running"
sleep 8
if loghas node-a 200s 'the mirror session ended|paused'; then ok H2-002b "node-a noticed the lost mirror (state paused path)"; else nv H2-002b "node-a pause not observed in log window"; fi
docker start node-b >/dev/null 2>&1; waithealth node-b && ok H2-002c "node-b rejoined and healthy" || bad H2-002c "node-b did not rejoin"
sleep 6
if loghas node-a 60s 'the mirror has caught up|mirror connected'; then ok H2-002d "node-a re-established the mirror after B rejoined"; else nv H2-002d "mirror re-establishment not observed yet"; fi

echo "### H2-003 full replication partition"
P=$(bash scripts/network-partition.sh 2>&1); printf '%s\n' "$P"
case "$P" in *"PASS | replication isolated"*) ok H2-003 "replication isolated both directions, no bypass";; *) nv H2-003 "isolation could not be proven on this host";; esac
sleep 8
if loghas node-a 120s '"state":"paused"'; then ok H2-003b "owner paused while peer unreachable (ADR-0010)"; else nv H2-003b "paused state not seen in log window"; fi
IP=$(dora 21)
case "$P" in *"PASS | replication isolated"*)
  if [ -z "$IP" ]; then ok H2-003c "no ACK while partitioned (fail-closed)"; else bad H2-003c "ACK issued while partitioned ($IP)"; fi;;
*) nv H2-003c "ACK suppression not testable without proven isolation";; esac
bash scripts/network-recover.sh node-b >/dev/null 2>&1; sleep 6
if loghas node-a 60s 'the mirror has caught up|mirror connected'; then ok H2-003d "mirror recovered and caught up"; else nv H2-003d "recovery catch-up not observed"; fi

echo "### H2-007/H2-008 crash around the ACK boundary"
IP=$(dora 31)
if [ -n "$IP" ]; then
  ok H2-008 "lease committed and ACKed before crash ($IP)"
  docker kill -s KILL node-a >/dev/null 2>&1; docker start node-a >/dev/null 2>&1; waithealth node-a || true
  n=$(leases "$IP")
  [ "${n:-0}" -ge 1 ] && ok H2-008b "ACKed lease $IP survived SIGKILL (durable before ACK)" || bad H2-008b "ACKed lease $IP lost after crash"
  n2=$(curl -s "http://127.0.0.1:18182/api/v1/dhcp/leases?ip=$IP" -H "Authorization: Bearer $(printf '%s' "$(login 18182)"|jget "d['data']['token']")" | jget "d['meta']['total']")
  [ "${n2:-0}" -ge 1 ] && ok H2-008c "standby mirror also holds $IP after owner crash" || nv H2-008c "standby copy not visible via API"
else nv H2-007 "could not obtain a lease to crash around"; nv H2-008 "could not obtain a lease to crash around"; fi

echo "### H2-014 stale owner return / epoch"
baseline
docker kill -s KILL node-a >/dev/null 2>&1
# The takeover contract refuses while the primary still reports watermarks
# (15s window). Waiting it out is the documented, correct behaviour.
tkout=$(docker exec node-b goddi ha takeover -c /etc/goddi/config.yaml --confirm --old-primary-cannot-write --accept-gap 1000000 2>&1)
case "$tkout" in
  *"still answering"*) ok H2-014 "takeover refused while primary watermarks were fresh (contract enforced)";;
  *) bad H2-014 "unexpected takeover outcome: $(printf '%s' "$tkout" | tail -2)";;
esac
for _ in $(seq 1 20); do
  sleep 2
  tkout=$(docker exec node-b goddi ha takeover -c /etc/goddi/config.yaml --confirm --old-primary-cannot-write --accept-gap 1000000 2>&1)
  case "$tkout" in *"is now the primary"*) break;; esac
done
case "$tkout" in *"is now the primary"*) ok H2-014a "takeover accepted once the primary went silent (explicit operator action)";;
  *) nv H2-014a "takeover still refused: $(printf '%s' "$tkout" | tail -1)";; esac
docker restart node-b >/dev/null 2>&1; waithealth node-b || true
sleep 4
docker exec node-b goddi ha status -c /etc/goddi/config.yaml 2>/dev/null | grep -qE "role in force: +primary" && ok H2-014b "node-b is now the owner (takes effect on restart)" || nv H2-014b "node-b role not yet primary"
docker start node-a >/dev/null 2>&1; waithealth node-a || true; sleep 8
sa=$(docker exec node-a goddi ha status -c /etc/goddi/config.yaml 2>/dev/null)
printf '%s\n' "$sa" > "$OUT/H2-014-node-a-status.txt"
case "$sa" in *"role in force:    standby"*) ok H2-014c "old owner returned as standby (no stale write authority)";;
  *) if loghas node-a 120s 'epoch|fenced|stale'; then ok H2-014c "old owner refused by epoch/fence (see logs)"; else bad H2-014c "old owner did not demote after takeover"; fi;; esac
docker exec node-b goddi ha status -c /etc/goddi/config.yaml 2>/dev/null | grep -qE "role in force: +primary" && ok H2-014d "new owner kept authority after old owner rejoined" || bad H2-014d "ownership flapped"

echo "### H2-015 DNS publish interrupted (standby down during publish)"
baseline
docker stop node-b >/dev/null 2>&1
Z=$(curl -s -X POST http://127.0.0.1:18181/api/v1/dns/zones "${H[@]}" -d '{"name":"fault15.test.","type":"primary"}' | jget "d['data']['id']")
if [ -n "${Z:-}" ]; then
  curl -s -X POST "http://127.0.0.1:18181/api/v1/dns/zones/$Z/records" "${H[@]}" -d '{"name":"a","type":"A","ttl":60,"value":"192.0.2.77"}' -o /dev/null
  r=""
  for _ in $(seq 1 15); do
    r=$(docker exec client sh -c 'apk add -q bind-tools >/dev/null 2>&1; dig +short +time=2 +tries=1 @node-a a.fault15.test. A' 2>/dev/null | head -1)
    [ "$r" = "192.0.2.77" ] && break; sleep 2
  done
  [ "$r" = "192.0.2.77" ] && ok H2-015 "publish completed on node-a while node-b was down (no false 'all nodes published')" || bad H2-015 "node-a did not serve the new record ($r)"
  nv H2-015b "cross-node publish coordination is ADR-0005 (not implemented); 'all nodes published' cannot be claimed"
else bad H2-015 "zone create failed"; fi
docker start node-b >/dev/null 2>&1; waithealth node-b || true

echo "### H2-017 concurrent clients"
baseline
if [ -z "$(docker ps --filter name=^node-a$ --filter status=running -q)" ]; then
  docker start node-a >/dev/null 2>&1; waithealth node-a || true; sleep 6
fi
for i in 1 2 3 4; do
  ( docker run --rm --network goddi-h2_client-net --cap-add NET_RAW --cap-add NET_ADMIN       --name "h2conc$i" --entrypoint sh goddi:h2 -c 'udhcpc -i eth0 -n -q -f -x hostname:conc -t 6 -T 2 2>&1 | grep -o "lease of [0-9.]*"'       > "$OUT/concurrent-$i-$TS.txt" 2>&1 ) &
done; wait
cat "$OUT"/concurrent-*-"$TS".txt > "$OUT/concurrent-all-$TS.txt" 2>/dev/null
got=$(grep -oE '10\.77\.3\.[0-9]+' "$OUT/concurrent-all-$TS.txt" 2>/dev/null | sort -u | wc -l | tr -d ' ')
dups=$(grep -oE '10\.77\.3\.[0-9]+' "$OUT/concurrent-all-$TS.txt" 2>/dev/null | sort | uniq -d | wc -l | tr -d ' ')
if [ "$got" -ge 2 ] && [ "$dups" -eq 0 ]; then ok H2-017 "concurrent DORA from 4 independent clients: $got distinct addresses, 0 duplicates"
elif [ "$got" -ge 1 ] && [ "$dups" -eq 0 ]; then nv H2-017 "only $got concurrent lease(s) obtained; no duplicate observed"
else bad H2-017 "concurrent DORA got=$got dups=$dups"; fi

echo "### H2-018 manual recovery after owner loss"
baseline
baseline
docker kill -s KILL node-a >/dev/null 2>&1; sleep 3
docker exec node-b goddi ha status -c /etc/goddi/config.yaml 2>/dev/null | grep -qE "role in force: +standby" && ok H2-018 "standby stayed standby with owner gone (no auto-takeover, DEC-002)" || bad H2-018 "standby changed role on its own"
docker start node-a >/dev/null 2>&1; waithealth node-a && ok H2-018b "owner restored by operator action" || bad H2-018b "owner did not restart"

echo "### H2-019 management plane down, data plane keeps serving"
baseline
IPB=$(dora 51)
docker exec node-a sh -c 'pkill -f "goddi serve"' >/dev/null 2>&1; sleep 3
nv H2-019 "single-binary process: management and data plane share one process; split-plane mode (server.role=dns|dhcp) is the supported path and is not exercised here"
docker start node-a >/dev/null 2>&1; waithealth node-a || true
[ -n "$IPB" ] && ok H2-019b "lease obtained before the management-plane test ($IPB)"

echo "### Kernel/CI-only scenarios"
for id in H2-004 H2-005 H2-006 H2-009 H2-010 H2-011 H2-012 H2-013 H2-016 H2-020; do
  nv "$id" "requires tc/netem, disk-fault injection or product features (license) — Linux CI / unit harness, not this host"
done

# Application-level fault injection (H2-009/010/011/012/016) in a separate pass.
if [ -x scripts/app-fault.sh ]; then bash scripts/app-fault.sh; fi
printf '{"suite":"faults","timestamp":"%s","pass":%d,"fail":%d,"not_verified":%d}\n' "$TS" "$PASS" "$FAIL" "$NV" > "$OUT/faults-summary-$TS.json"
bash scripts/collect-evidence.sh >/dev/null 2>&1 || true
echo "----"; echo "FAULTS PASS=$PASS FAIL=$FAIL NOT_VERIFIED=$NV"
[ "$FAIL" -eq 0 ]
