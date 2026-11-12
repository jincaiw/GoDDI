#!/bin/sh
# Resolve the client-network interface (the one on 10.77.3.0/24) at container
# start and pin dhcp.interfaces to it, so the DHCP server's serverIP matches the
# scope subnet regardless of Docker's ethN ordering across restarts.
set -e
CFG=/etc/goddi/config.yaml
IFACE=""
for d in /sys/class/net/*; do
  n=$(basename "$d")
  ip=$(ip -4 -o addr show dev "$n" 2>/dev/null | awk '{print $4}' | cut -d/ -f1)
  case "$ip" in 10.77.3.*) IFACE="$n"; break;; esac
done
if [ -n "$IFACE" ]; then
  sed "s/interfaces: \[\]/interfaces: [\"$IFACE\"]/" "$CFG" > /tmp/config.yaml
  echo "h2-entrypoint: DHCP client interface = $IFACE" >&2
else
  cp "$CFG" /tmp/config.yaml
  echo "h2-entrypoint: no 10.77.3.x interface found; leaving auto-detect" >&2
fi
exec goddi serve --config /tmp/config.yaml
