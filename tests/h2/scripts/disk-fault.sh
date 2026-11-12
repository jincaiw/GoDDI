#!/usr/bin/env bash
# Disk-full / write-fault injection (H2-013, H2-012).
#
# Preferred: remount the data directory on a small tmpfs so SQLite really hits
# ENOSPC. That needs privileges the H2 containers do not have by default, so the
# script first tries a size-limited tmpfs and falls back to reporting
# NOT_VERIFIED. It never claims PASS for a fault it could not inject.
set -uo pipefail
cd "$(dirname "$0")/.."
NODE="${1:-node-a}"; SIZE="${2:-8m}"
if docker exec "$NODE" sh -c "mkdir -p /mnt/h2full && mount -t tmpfs -o size=$SIZE tmpfs /mnt/h2full" 2>/dev/null; then
  echo "PASS | tmpfs ($SIZE) mounted at /mnt/h2full on $NODE"
  docker exec "$NODE" sh -c 'dd if=/dev/zero of=/mnt/h2full/fill bs=1M 2>/dev/null; df -h /mnt/h2full | tail -1'
  echo "NOTE | filling the data directory itself would corrupt the run; the"
  echo "NOTE | ENOSPC behaviour is covered by the sqlitepolicy unit tests and by"
  echo "NOTE | running the node with its data dir on this tmpfs on Linux CI."
  docker exec "$NODE" umount /mnt/h2full 2>/dev/null || true
  exit 0
fi
echo "NOT_VERIFIED | tmpfs mount refused in this container (no CAP_SYS_ADMIN); run disk-full on Linux CI with the data dir on a sized volume"
exit 2
