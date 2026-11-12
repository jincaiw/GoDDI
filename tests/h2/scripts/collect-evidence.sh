#!/usr/bin/env bash
# Collect logs, container state, HA status and network attachments as evidence.
set -uo pipefail
cd "$(dirname "$0")/.."
TS=$(date -u +%Y%m%dT%H%M%SZ); D="reports/$TS"; mkdir -p "$D"
for n in node-a node-b client relay fault-injector observer; do
  docker logs "$n" > "$D/$n.log" 2>&1 || true
  docker inspect "$n" > "$D/$n.inspect.json" 2>/dev/null || true
done
for n in node-a node-b; do
  docker exec "$n" wget -qO- http://localhost:6080/api/v1/dhcp/ha > "$D/$n-ha.json" 2>/dev/null || true
done
docker network ls > "$D/networks.txt" 2>&1 || true
docker compose -f docker-compose.yml ps > "$D/compose-ps.txt" 2>&1 || true
echo "evidence collected in $D"; ls -1 "$D"
