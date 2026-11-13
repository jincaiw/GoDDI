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
# Protocol-level acceptance suites (run when present).
if [ -x scripts/dns-acceptance.sh ]; then bash scripts/dns-acceptance.sh; [ $? -ne 0 ] && rc=1; fi
if [ -x scripts/dhcp-acceptance.sh ]; then bash scripts/dhcp-acceptance.sh; [ $? -ne 0 ] && rc=1; fi
if [ -x scripts/ipam-acceptance.sh ]; then bash scripts/ipam-acceptance.sh; [ $? -ne 0 ] && rc=1; fi
if [ -x scripts/dns-transfer-acceptance.sh ]; then bash scripts/dns-transfer-acceptance.sh; [ $? -ne 0 ] && rc=1; fi
if [ -x scripts/config-author-acceptance.sh ]; then bash scripts/config-author-acceptance.sh; [ $? -ne 0 ] && rc=1; fi
bash scripts/collect-evidence.sh
bash scripts/h2-report.sh
exit $rc
