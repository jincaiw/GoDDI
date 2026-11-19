#!/usr/bin/env bash
# Print a client-net fixture address that does not collide with live nodes.
# Docker reassigns client-net addresses across recreates, so scripts must never
# hard-code 10.77.3.3: after an aggregate run that address can belong to node-a.
set -eu
used=" $(for n in node-a node-b relay fault-injector observer client; do
  docker inspect -f "{{range .NetworkSettings.Networks}}{{.IPAddress}} {{end}}" "$n" 2>/dev/null || true
 done | tr " " "\n" | grep "^10\.77\.3\." | sort -u | tr "\n" " ") "
for host in 200 201 202 203 204 205 206 207 208 209 210; do
  addr="10.77.3.$host"
  case "$used" in
    *" $addr "*) continue ;;
  esac
  echo "$addr"
  exit 0
done
echo "no free client-net fixture address" >&2
exit 1
