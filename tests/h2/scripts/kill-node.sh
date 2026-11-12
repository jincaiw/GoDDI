#!/usr/bin/env bash
# SIGKILL a node (H2-001/H2-002). Use --restart to bring it back.
set -uo pipefail
NODE="${1:?usage: kill-node.sh <node-a|node-b> [--restart]}"
docker kill -s KILL "$NODE" >/dev/null 2>&1 || { echo "FAIL | could not kill $NODE"; exit 1; }
echo "KILLED | $NODE (SIGKILL)"
if [ "${2:-}" = "--restart" ]; then docker start "$NODE" >/dev/null && echo "RESTARTED | $NODE"; fi
