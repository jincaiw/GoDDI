#!/usr/bin/env bash
# Entry: bring-up assumed; wait healthy then run consistency checks + evidence.
# Use the base pair (docker-compose.yml) here. dns-primary-secondary cannot
# install a secondary where the demo overlay has already replicated the zone.
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
if [ -x scripts/dns-primary-secondary-acceptance.sh ]; then bash scripts/dns-primary-secondary-acceptance.sh; [ $? -ne 0 ] && rc=1; fi
if [ -x scripts/auto-failover-drill.sh ]; then bash scripts/auto-failover-drill.sh; [ $? -ne 0 ] && rc=1; fi
if [ -x scripts/config-replication-acceptance.sh ]; then bash scripts/config-replication-acceptance.sh; [ $? -ne 0 ] && rc=1; fi
if [ -x scripts/ipam-lifecycle-replication.sh ]; then bash scripts/ipam-lifecycle-replication.sh; [ $? -ne 0 ] && rc=1; fi
if [ -x scripts/split-plane-acceptance.sh ]; then bash scripts/split-plane-acceptance.sh; [ $? -ne 0 ] && rc=1; fi
bash scripts/collect-evidence.sh
bash scripts/h2-report.sh
exit $rc
