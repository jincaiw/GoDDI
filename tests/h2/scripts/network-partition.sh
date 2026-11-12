#!/usr/bin/env bash
# Isolate a node from the replication network (default: node-b). Verifies the
# fault scope by confirming the node has no repl-net attachment afterwards.
set -uo pipefail
cd "$(dirname "$0")/.."
NODE="${1:-node-b}"
NET=goddi-h2_repl-net
if docker network disconnect "$NET" "$NODE" 2>/dev/null; then echo "PARTITION: $NODE disconnected from $NET"; else echo "PARTITION: $NODE already disconnected"; fi
if docker inspect -f '{{range $k,$v := .NetworkSettings.Networks}}{{$k}} {{end}}' "$NODE" | grep -q "repl-net"; then
  echo "FAIL | $NODE still attached to repl-net"; exit 1
fi
echo "PASS | $NODE isolated from repl-net (no bypass path on repl-net)"
