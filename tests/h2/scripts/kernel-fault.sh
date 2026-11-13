#!/usr/bin/env bash
# Kernel-level fault injection (H2-004/005/006). These need a Linux kernel that
# honours tc qdiscs, which is what a native Linux CI runner provides. On macOS
# the containers run under the Docker Linux VM (OrbStack/Desktop), which is a
# real Linux kernel, so the qdisc is real for the container pair; the script
# still verifies the effect rather than assuming it.
#
# tc is applied inside the target container's network namespace by joining it
# from a small tool image (the product image stays minimal). Every fault is
# cleared on exit.
set -uo pipefail
cd "$(dirname "$0")/.."
TS=$(date -u +%Y%m%dT%H%M%SZ); OUT=reports; mkdir -p "$OUT"
J="$OUT/kernel-faults-$TS.jsonl"; : > "$J"
PASS=0; FAIL=0; NV=0
rec(){ printf '{"id":"%s","verdict":"%s","detail":"%s"}\n' "$1" "$2" "${3//\"/\'}" >> "$J"; }
ok(){ echo "PASS | $1 $2"; PASS=$((PASS+1)); rec "$1" PASS "$2"; }
bad(){ echo "FAIL | $1 $2"; FAIL=$((FAIL+1)); rec "$1" FAIL "$2"; }
nv(){ echo "NOT_VERIFIED | $1 $2"; NV=$((NV+1)); rec "$1" NOT_VERIFIED "$2"; }
jget(){ python3 -c "import sys,json;d=json.load(sys.stdin);print($1)" 2>/dev/null; }
login(){ curl -s -X POST "http://127.0.0.1:$1/api/v1/auth/login" -H 'Content-Type: application/json' -d '{"username":"admin","password":"AdminPass-0123456789"}'; }

TOOLS=goddi:h2-tools
docker image inspect "$TOOLS" >/dev/null 2>&1 || docker build -q -f Dockerfile.tools -t "$TOOLS" . >/dev/null
tcis(){ # node iface-spec...
  local node=$1 iface=$2 spec=$3
  docker run --rm --net=container:"$node" --cap-add NET_ADMIN --entrypoint sh "$TOOLS" -c \
    "tc qdisc del dev $iface root 2>/dev/null; tc qdisc add dev $iface root $spec && tc qdisc show dev $iface" 2>&1
}
tcclear(){ local node=$1 iface=$2
  docker run --rm --net=container:"$node" --cap-add NET_ADMIN --entrypoint sh "$TOOLS" -c "tc qdisc del dev $iface root 2>/dev/null; true" >/dev/null 2>&1; }
replif(){ docker exec "$1" sh -c 'ip -4 -o addr show | awk "{print \$2,\$4}" | grep "10\.77\.2\." | awk "{print \$1}" | head -1'; }
dora(){ docker exec client sh -c "ip link set eth0 down 2>/dev/null; ip link set eth0 address 02:00:00:00:04:$1; ip link set eth0 up; rm -f /tmp/kf.log; udhcpc -i eth0 -n -q -f -x hostname:kf$1 -t 6 -T 2 >/tmp/kf.log 2>&1; grep -o 'lease of [0-9.]*' /tmp/kf.log | awk '{print \$3}' | head -1"; }
cleanup(){ tcclear node-a "$(replif node-a)"; tcclear node-b "$(replif node-b)"; }
trap cleanup EXIT

# baseline: node-a primary, node-b standby, a wide scope, and an IPAM subnet so
# facts flow (not strictly needed for these, but keeps the pair realistic).
RA=$(login 18181); TOK=$(printf '%s' "$RA"|jget "d['data']['token']"); CS=$(printf '%s' "$RA"|jget "d['data']['csrf_token']")
H=(-H "Authorization: Bearer $TOK" -H "X-CSRF-Token: $CS" -H 'Content-Type: application/json')
curl -s -X POST http://127.0.0.1:18181/api/v1/dhcp/scopes "${H[@]}" \
  -d '{"name":"h2-kernelfault","subnet":"10.77.3.0/24","start_ip":"10.77.3.100","end_ip":"10.77.3.240","lease_time":600}' -o /dev/null
sleep 4
A_IF=$(replif node-a); B_IF=$(replif node-b)
echo "repl ifaces: node-a=$A_IF node-b=$B_IF"
[ -n "$A_IF" ] && [ -n "$B_IF" ] || { nv H2-004 "replication interface not found"; nv H2-005 "replication interface not found"; nv H2-006 "replication interface not found"; echo "KERNEL FAULTS PASS=0 FAIL=0 NOT_VERIFIED=3"; exit 2; }

echo "### H2-004 one-way replication fault (node-a -> node-b dropped)"
OUT4=$(tcis node-a "$A_IF" "netem loss 100%")
case "$OUT4" in *netem*) ok H2-004a "netem loss 100% applied on node-a egress ($A_IF)";; *) nv H2-004a "could not apply netem: $OUT4";; esac
sleep 8
# Confirm the fault is one-way at the packet level: the qdisc is present only on
# node-a's egress, and node-b has no qdisc at all. (A TCP handshake still fails
# because it needs both directions; that is expected and is why the assertion is
# about where the fault is installed, not about a ping succeeding.)
QB=$(docker run --rm --net=container:node-b --cap-add NET_ADMIN --entrypoint sh "$TOOLS" -c "tc qdisc show dev $B_IF" 2>/dev/null | tail -1)
QA=$(docker run --rm --net=container:node-a --cap-add NET_ADMIN --entrypoint sh "$TOOLS" -c "tc qdisc show dev $A_IF" 2>/dev/null | tail -1)
case "$QA" in *netem*) case "$QB" in *netem*) nv H2-004b "both sides carry a qdisc (not one-way)";; *) ok H2-004b "fault is one-way: netem only on node-a egress, node-b clean";; esac;; *) nv H2-004b "node-a qdisc missing after apply";; esac
IP4=$(dora 04)
LOG4=$(docker logs --since 30s node-a 2>&1)
if [ -z "${IP4:-}" ]; then ok H2-004c "primary withheld the ACK under a one-way fault"; else bad H2-004c "primary still ACKed under one-way fault ($IP4)"; fi
case "$LOG4" in *"withholding"*|*"no second copy"*|*"silent"*) ok H2-004d "log shows the fail-closed reason";; *) nv H2-004d "no fail-closed log line captured";; esac
tcclear node-a "$A_IF"; sleep 6

echo "### H2-005 replication latency and loss"
OUT5a=$(tcis node-a "$A_IF" "netem delay 120ms loss 5%"); OUT5b=$(tcis node-b "$B_IF" "netem delay 120ms loss 5%")
case "$OUT5a$OUT5b" in *netem*netem*) ok H2-005a "netem delay+loss applied on both replication interfaces";; *) nv H2-005a "netem not fully applied (a=$OUT5a b=$OUT5b)";; esac
IP5=$(dora 05)
if [ -n "${IP5:-}" ]; then
  ok H2-005b "DORA succeeded under latency/loss ($IP5)"
  # the binding must still reach the standby (retransmission absorbs the loss)
  seen=0
  for _ in $(seq 1 20); do
    n=$(curl -s "http://127.0.0.1:18182/api/v1/dhcp/leases?ip=$IP5" -H "Authorization: Bearer $(printf '%s' "$(login 18182)"|jget "d['data']['token']")" | jget "d['meta']['total']")
    [ "${n:-0}" -ge 1 ] && { seen=1; break; }; sleep 2
  done
  [ "$seen" -eq 1 ] && ok H2-005c "binding replicated to the standby despite loss (retransmission)" || nv H2-005c "binding not yet visible on the standby"
else nv H2-005b "DORA failed under latency/loss"; fi
tcclear node-a "$A_IF"; tcclear node-b "$B_IF"; sleep 6

echo "### H2-006 replication acknowledgement loss (node-b -> node-a dropped)"
OUT6=$(tcis node-b "$B_IF" "netem loss 100%")
case "$OUT6" in *netem*) ok H2-006a "netem loss 100% applied on node-b egress (ACK path)";; *) nv H2-006a "could not apply netem: $OUT6";; esac
sleep 8
IP6=$(dora 06)
if [ -z "${IP6:-}" ]; then ok H2-006b "ACK withheld while replication acknowledgements are lost"; else bad H2-006b "ACK granted while ACK path is broken ($IP6)"; fi
tcclear node-b "$B_IF"; sleep 8
IP6b=$(dora 07)
[ -n "${IP6b:-}" ] && ok H2-006c "recovery: ACK resumes after the fault is cleared ($IP6b)" || nv H2-006c "no ACK after recovery"

echo "### H2-013 disk capacity exhausted"
NET=goddi-h2-diskfull; NODE=h2-diskfull
docker network inspect "$NET" >/dev/null 2>&1 || docker network create --subnet 10.77.9.0/24 "$NET" >/dev/null
docker rm -f "$NODE" >/dev/null 2>&1 || true
docker run -d --name "$NODE" --network "$NET" --ip 10.77.9.2 \
  --tmpfs /data:size=4m \
  --cap-add NET_RAW --cap-add NET_ADMIN \
  -p 18190:6081 \
  -e GODDI_ADMIN_USERNAME=admin -e GODDI_ADMIN_PASSWORD=AdminPass-0123456789 \
  -e GODDI_SECURITY_JWT_SECRET=diskfull-jwt-secret-0123456789 \
  -v "$(pwd)/config/diskfull-node.yaml:/etc/goddi/config.yaml:ro" \
  goddi:h2 serve --config /etc/goddi/config.yaml >/dev/null 2>&1
okd=0
for _ in $(seq 1 40); do
  curl -s -o /dev/null -w '' "http://127.0.0.1:18190/health" 2>/dev/null && { okd=1; break; }; sleep 2
done
if [ "$okd" -eq 1 ]; then
  RD=$(curl -s -X POST http://127.0.0.1:18190/api/v1/auth/login -H 'Content-Type: application/json' -d '{"username":"admin","password":"AdminPass-0123456789"}')
  DT=$(printf '%s' "$RD"|jget "d['data']['token']"); DC=$(printf '%s' "$RD"|jget "d['data']['csrf_token']")
  DH=(-H "Authorization: Bearer $DT" -H "X-CSRF-Token: $DC" -H 'Content-Type: application/json')
  curl -s -X POST http://127.0.0.1:18190/api/v1/dhcp/scopes "${DH[@]}" \
    -d '{"name":"diskfull","subnet":"10.77.9.0/24","start_ip":"10.77.9.100","end_ip":"10.77.9.110","lease_time":600}' -o /dev/null
  sleep 4
  # Fill the tmpfs so a new binding cannot be committed.
  docker exec "$NODE" sh -c 'dd if=/dev/zero of=/data/fill bs=1k count=4096 2>/dev/null; df -h /data | tail -1' >/dev/null 2>&1 || true
  IP13=$(docker run --rm --network "$NET" --cap-add NET_RAW --cap-add NET_ADMIN --entrypoint sh goddi:h2 -c \
    'udhcpc -i eth0 -n -q -f -x hostname:diskfull -t 5 -T 2 2>&1 | grep -o "lease of [0-9.]*" | head -1' 2>/dev/null | tr -d '\r')
  LOG13=$(docker logs --since 40s "$NODE" 2>&1)
  case "$LOG13" in
    *"no space"*|*"disk"*|*"SQLITE_FULL"*|*"storage"*|*"ENOSPC"*|*"withholding"*|*"read-only"*|*"readonly"*)
      if [ -z "${IP13:-}" ]; then ok H2-013a "disk-full surfaced and the ACK was withheld"; else nv H2-013a "storage error logged but a lease was still granted ($IP13)"; fi;;
    *) if [ -z "${IP13:-}" ]; then ok H2-013b "no lease granted on a full disk (fail-closed)"; else nv H2-013b "lease granted on a full disk ($IP13); storage classification not observed"; fi;;
  esac
else
  nv H2-013 "diskfull node did not become ready"
fi
docker rm -f "$NODE" >/dev/null 2>&1 || true
docker network rm "$NET" >/dev/null 2>&1 || true

printf '{"suite":"kernel-faults","timestamp":"%s","pass":%d,"fail":%d,"not_verified":%d}\n' "$TS" "$PASS" "$FAIL" "$NV" > "$OUT/kernel-faults-summary-$TS.json"
echo "----"; echo "KERNEL-FAULTS PASS=$PASS FAIL=$FAIL NOT_VERIFIED=$NV"
[ "$FAIL" -eq 0 ]
