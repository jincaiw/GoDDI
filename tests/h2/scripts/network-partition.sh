#!/usr/bin/env bash
# Isolate the replication network (default: full A/B partition). Docker Desktop
# routes between bridges via the host, so detaching repl-net alone is not enough
# (a bypass path remains). We therefore add a blackhole route for the
# replication subnet on BOTH nodes and VERIFY the bypass is gone. On macOS the
# kernel-level isolation may still differ from Linux; the verification below is
# what decides PASS vs NOT_VERIFIED, never an assumption.
set -uo pipefail
cd "$(dirname "$0")/.."
MODE="${1:-full}"; SUB=10.77.2.0/24
bh_add(){ docker exec "$1" ip route add blackhole "$SUB" 2>/dev/null || true; }
bh_del(){ docker exec "$1" ip route del blackhole "$SUB" 2>/dev/null || true; }
if [ "$MODE" = "recover" ]; then bh_del node-a; bh_del node-b; echo "RECOVER: blackhole removed on node-a/node-b"; exit 0; fi
docker network disconnect goddi-h2_repl-net node-b 2>/dev/null || true
bh_add node-a; bh_add node-b
fail=0
docker exec node-a sh -c 'nc -z -w2 10.77.2.12 9953' 2>/dev/null && { echo "FAIL | bypass: node-a -> node-b:9953 still reachable"; fail=1; }
docker exec node-b sh -c 'nc -z -w2 10.77.2.11 9953' 2>/dev/null && { echo "FAIL | bypass: node-b -> node-a:9953 still reachable"; fail=1; }
if [ "$fail" -eq 0 ]; then echo "PASS | replication isolated both directions (no bypass path)"; else echo "NOT_VERIFIED | kernel/bridge bypass remains; run on Linux CI"; fi
exit "$fail"
