#!/usr/bin/env bash
# Replication-network latency/loss injection (H2-005) and one-way faults
# (H2-004), applied with tc/netem on the node's replication interface.
#
# tc/netem needs NET_ADMIN plus a kernel that honours qdiscs. Docker Desktop on
# macOS runs containers in a Linux VM: the qdisc applies to the VM's veth, so it
# is real for the container pair, but the host-side path is not the CI path.
# When the qdisc cannot be verified the scenario reports NOT_VERIFIED, never
# PASS.
set -uo pipefail
cd "$(dirname "$0")/.."
ACTION="${1:-apply}"; NODE="${2:-node-b}"; DELAY="${3:-200ms}"; LOSS="${4:-20%}"
IFACE=$(docker exec "$NODE" sh -c 'ip -4 -o addr show | awk "{print \$2, \$4}" | grep "^.*10\.77\.2\." | awk "{print \$1}" | head -1')
if [ -z "${IFACE:-}" ]; then echo "FAIL | could not find the replication interface on $NODE"; exit 1; fi
if [ "$ACTION" = "clear" ]; then
  docker exec "$NODE" tc qdisc del dev "$IFACE" root 2>/dev/null || true
  echo "RECOVER | qdisc cleared on $NODE/$IFACE"
  exit 0
fi
docker exec "$NODE" tc qdisc del dev "$IFACE" root 2>/dev/null || true
if ! docker exec "$NODE" tc qdisc add dev "$IFACE" root netem delay "$DELAY" loss "$LOSS" 2>/dev/null; then
  echo "NOT_VERIFIED | tc/netem could not be applied on $NODE/$IFACE (kernel/qdisc unavailable)"; exit 2
fi
OUT=$(docker exec "$NODE" tc qdisc show dev "$IFACE" 2>/dev/null)
case "$OUT" in
  *netem*) echo "PASS | netem active on $NODE/$IFACE: $OUT";;
  *) echo "NOT_VERIFIED | qdisc applied but not visible: $OUT"; exit 2;;
esac
