#!/usr/bin/env bash
# Reattach a node to the replication network at its fixed IP.
set -uo pipefail
cd "$(dirname "$0")/.."
NODE="${1:-node-b}"
case "$NODE" in node-a) IP=10.77.2.11;; node-b) IP=10.77.2.12;; *) echo "unknown node $NODE"; exit 2;; esac
if docker network connect --ip "$IP" goddi-h2_repl-net "$NODE" 2>/dev/null; then echo "RECOVER: $NODE reattached at $IP"; else echo "RECOVER: $NODE already attached"; fi
