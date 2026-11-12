#!/usr/bin/env bash
set -uo pipefail
cd "$(dirname "$0")/.."
NODE="${1:-node-b}"
case "$NODE" in node-a) IP=10.77.2.11;; node-b) IP=10.77.2.12;; *) echo "unknown node $NODE"; exit 2;; esac
docker exec node-a ip route del blackhole 10.77.2.0/24 2>/dev/null || true
docker exec node-b ip route del blackhole 10.77.2.0/24 2>/dev/null || true
if docker network connect --ip "$IP" goddi-h2_repl-net "$NODE" 2>/dev/null; then echo "RECOVER: $NODE reattached at $IP"; else echo "RECOVER: $NODE already attached"; fi
