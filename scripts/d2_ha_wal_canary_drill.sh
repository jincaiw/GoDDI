#!/usr/bin/env bash
# D2 HA canary drill: switch ONE of two HA nodes (the primary) to the WAL store
# while the standby stays sqlite, then exercise crash + rollback.
#
# Prerequisite: the docker-compose.ha.yml volumes (primary-data/standby-data)
# already contain the ha-bridge scope, ha.test zone and ha-net-space IPAM mapping
# (seeded by the earlier HA exercise rounds). The script ends by rolling the
# primary back to sqlite, restoring the default.
set -uo pipefail
HA=docker-compose.ha.yml; OV=docker-compose.d2-canary.yml; PASS=0; FAIL=0
ok(){ echo "PASS | $*"; PASS=$((PASS+1)); }; bad(){ echo "FAIL | $*"; FAIL=$((FAIL+1)); }
up_wal(){ docker compose -f $HA -f $OV up -d --force-recreate goddi-primary goddi-standby >/dev/null 2>&1; }
up_sql(){ docker compose -f $HA up -d --force-recreate goddi-primary goddi-standby >/dev/null 2>&1; }
wait_api(){ for i in $(seq 1 80); do curl -fsS http://127.0.0.1:$1/health >/dev/null 2>&1 && return 0; sleep 0.5; done; return 1; }
psql(){ docker exec -u root "$1" sh -c "command -v sqlite3 >/dev/null 2>&1 || apk add -q sqlite >/dev/null 2>&1; sqlite3 $2 \"$3\""; }
wait_cnt(){ local c="$1" want="$2"; for i in $(seq 1 80); do v=$(eval "$c"); [ "$v" = "$want" ] && return 0; sleep 0.5; done; return 1; }
run_client(){ docker rm -f "$1" >/dev/null 2>&1; docker run -d --name "$1" --network ha-net --cap-add NET_ADMIN alpine:3.24 sh -c "udhcpc -i eth0 -n -f -x hostname:$2 >/tmp/d.log 2>&1; sleep 120" >/dev/null 2>&1; sleep 13; docker exec "$1" sh -c "tail -1 /tmp/d.log"; }

echo "== bring up HA pair: primary wal, standby sqlite =="; up_wal; wait_api 18081 && ok "primary wired"; wait_api 18082 && ok "standby wired"
[ "$(docker logs goddi-primary 2>&1 | grep -c 'event-sourced WAL')" -ge 1 ] && ok "primary runs WAL store" || bad "primary not WAL"
[ "$(docker logs goddi-standby 2>&1 | grep -c 'event-sourced WAL')" = "0" ] && ok "standby stays sqlite" || bad "standby unexpectedly WAL"
sleep 6
echo "== real DORA through WAL primary =="; run_client d2hac ha-canary | sed 's/^/  /'
IP=$(docker exec d2hac sh -c "grep -o 'lease of [0-9.]*' /tmp/d.log | awk '{print \$3}'"); echo "  ip=$IP"
[ -n "$IP" ] && ok "client obtained a lease from the WAL primary" || bad "no lease"
sleep 4
wait_cnt "psql goddi-primary /var/lib/goddi/leases.db \"SELECT COUNT(*) FROM dhcp_leases WHERE ip_address='$IP'\"" 1 && ok "primary lease row present" || bad "primary lease missing"
for i in $(seq 1 60); do f=$(psql goddi-primary /var/lib/goddi/leases.db "SELECT COUNT(*) FROM dhcp_ipam_observation_events WHERE payload LIKE '%$IP%'"); [ "$f" -ge 1 ] && break; sleep 0.5; done; [ "${f:-0}" -ge 1 ] && ok "fact derived from WAL on primary" || bad "no fact on primary"
for i in $(seq 1 60); do dn=$(psql goddi-primary /var/lib/goddi/dnsdata.db "SELECT name FROM dns_records WHERE value='$IP'"); [ -n "$dn" ] && break; sleep 0.5; done; [ -n "${dn:-}" ] && ok "DNS record published on primary" || bad "no DNS record"
wait_cnt "psql goddi-standby /var/lib/goddi/leases.db \"SELECT COUNT(*) FROM dhcp_leases WHERE ip_address='$IP'\"" 1 && ok "lease mirrored to sqlite standby" || bad "standby mirror missing"

echo "== SIGKILL primary: fail-closed (standby must NOT auto-promote) =="; docker kill goddi-primary >/dev/null 2>&1; sleep 4
echo "  (standby role observed: $(docker exec goddi-standby goddi ha status -c /etc/goddi/config.yaml 2>/dev/null | grep -m1 'state:' | tr -s ' '))"
docker rm -f d2hac2 >/dev/null 2>&1; docker run -d --name d2hac2 --network ha-net --cap-add NET_ADMIN alpine:3.24 sh -c "timeout 8 udhcpc -i eth0 -n -f -q >/tmp/d.log 2>&1; grep -q 'lease of' /tmp/d.log" >/dev/null 2>&1; sleep 11
docker logs d2hac2 >/dev/null 2>&1; docker exec d2hac2 sh -c "grep -q 'lease of' /tmp/d.log" 2>/dev/null && bad "lease served with primary down" || ok "no lease while primary is down (fail-closed)"

echo "== restart WAL primary: replay + serve =="; docker start goddi-primary >/dev/null 2>&1; wait_api 18081 && ok "primary restarted"; sleep 6
wait_cnt "psql goddi-primary /var/lib/goddi/leases.db \"SELECT COUNT(*) FROM dhcp_leases WHERE ip_address='$IP'\"" 1 && ok "WAL replay kept the lease across SIGKILL" || bad "lease lost on crash"
run_client d2hac3 ha-canary3 >/dev/null; docker exec d2hac3 sh -c "grep -q 'lease of' /tmp/d.log" && ok "DORA served after crash/restart" || bad "no DORA after crash"

echo "== rollback primary to sqlite (same volume) =="; L=$(psql goddi-primary /var/lib/goddi/leases.db "SELECT COUNT(*) FROM dhcp_leases"); docker stop goddi-primary >/dev/null 2>&1; docker compose -f $HA up -d --force-recreate goddi-primary >/dev/null 2>&1; wait_api 18081 && ok "primary healthy after rollback"; sleep 6
[ "$(docker logs goddi-primary 2>&1 | grep -c 'event-sourced WAL')" = "0" ] && ok "WAL branch not engaged after rollback" || bad "wal still engaged"
[ "$(psql goddi-primary /var/lib/goddi/leases.db "SELECT COUNT(*) FROM dhcp_leases")" = "$L" ] && ok "leases preserved across rollback ($L)" || bad "lease count changed on rollback"
run_client d2hac4 ha-canary4 >/dev/null; docker exec d2hac4 sh -c "grep -q 'lease of' /tmp/d.log" && ok "DORA served after rollback to sqlite" || bad "no DORA after rollback"

echo "===================="; echo "PASS=$PASS FAIL=$FAIL"; [ "$FAIL" = "0" ] && { echo "DRILL: OK"; exit 0; } || { echo "DRILL: FAIL"; exit 1; }
