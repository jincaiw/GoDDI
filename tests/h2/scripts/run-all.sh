#!/usr/bin/env bash
# Entry: bring-up assumed; wait healthy then run consistency checks + evidence.
set -uo pipefail
cd "$(dirname "$0")/.."
for n in node-a node-b; do
  for _ in $(seq 1 30); do
    [ "$(docker inspect -f '{{.State.Health.Status}}' "$n" 2>/dev/null)" = healthy ] && break
    sleep 2
  done
done
bash scripts/verify-consistency.sh; rc=$?
bash scripts/collect-evidence.sh
exit $rc
