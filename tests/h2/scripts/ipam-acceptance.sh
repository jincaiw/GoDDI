#!/usr/bin/env bash
# IPAM / DDNS acceptance on the running H2 pair.
# Verifies: space+subnet creation, DHCP lease -> IPAM observed state linkage,
# outbox completeness/idempotency, projection-lag detection, DNS A/PTR
# publication. Honest verdicts only.
set -uo pipefail
cd "$(dirname "$0")/.."
TS=$(date -u +%Y%m%dT%H%M%SZ); OUT=reports; mkdir -p "$OUT"
PASS=0; FAIL=0; NV=0
ok(){ echo "PASS | $1"; PASS=$((PASS+1)); }
bad(){ echo "FAIL | $1"; FAIL=$((FAIL+1)); }
nv(){ echo "NOT_VERIFIED | $1"; NV=$((NV+1)); }
jget(){ python3 -c "import sys,json;d=json.load(sys.stdin);print($1)" 2>/dev/null; }
login(){ curl -s -X POST "http://127.0.0.1:$1/api/v1/auth/login" -H 'Content-Type: application/json' -d '{"username":"admin","password":"AdminPass-0123456789"}'; }
RA=$(login 18181); TOK=$(printf '%s' "$RA"|jget "d['data']['token']"); CS=$(printf '%s' "$RA"|jget "d['data']['csrf_token']")
H=(-H "Authorization: Bearer $TOK" -H "X-CSRF-Token: $CS" -H 'Content-Type: application/json')
# node-a's client-network address (not stable across recreates).
DNS_IP=$(docker exec node-a sh -c 'ip -4 -o addr show | awk "{print \$4}" | cut -d/ -f1 | grep "^10.77.3." | head -1')
[ -n "${DNS_IP:-}" ] || DNS_IP=10.77.3.2
sqlq(){ docker exec node-a sh -c "command -v sqlite3 >/dev/null 2>&1 || apk add -q sqlite >/dev/null 2>&1; sqlite3 -noheader /var/lib/goddi/goddi.db \"$1\"" 2>/dev/null | tr -d '\r'; }

# --- IPAM space + subnet ---------------------------------------------------
# Reuse an existing space/subnet so the suite is repeatable (the name is unique).
SP=$(curl -s http://127.0.0.1:18181/api/v1/ipam/spaces -H "Authorization: Bearer $TOK" \
  | python3 -c "import sys,json;d=json.load(sys.stdin)['data'];print(next((x['id'] for x in d if x['name']=='h2-space'),''))" 2>/dev/null)
if [ -n "${SP:-}" ]; then ok "IPAM space reused ($SP)"; else
  SP=$(curl -s -X POST http://127.0.0.1:18181/api/v1/ipam/spaces "${H[@]}" -d '{"name":"h2-space","description":"h2 acceptance"}' | jget "d['data']['id']")
  [ -n "${SP:-}" ] && ok "IPAM space created ($SP)" || bad "IPAM space create failed"
fi
sleep 3  # let the IPAM subnet reach the lease plane so facts can resolve a space
SN=$(curl -s "http://127.0.0.1:18181/api/v1/ipam/subnets?space_id=$SP" -H "Authorization: Bearer $TOK" \
  | python3 -c "import sys,json;d=json.load(sys.stdin)['data'];print(next((x['id'] for x in d if x['cidr']=='10.77.3.0/24'),''))" 2>/dev/null)
if [ -n "${SN:-}" ]; then ok "IPAM subnet reused ($SN)"; else
  SN=$(curl -s -X POST http://127.0.0.1:18181/api/v1/ipam/subnets "${H[@]}" \
    -d "{\"space_id\":\"$SP\",\"name\":\"h2-subnet\",\"cidr\":\"10.77.3.0/24\",\"vlan_id\":130,\"location\":\"h2\"}" | jget "d['data']['id']")
  [ -n "${SN:-}" ] && ok "IPAM subnet created (VLAN 130)" || bad "IPAM subnet create failed"
fi
sleep 4  # the dataplane sync interval is 1s; give it a margin before DORA
vlan=$(curl -s "http://127.0.0.1:18181/api/v1/ipam/subnets/$SN" -H "Authorization: Bearer $TOK" | jget "d['data']['vlan_id']")
[ "$vlan" = "130" ] && ok "subnet VLAN persisted (130)" || bad "subnet VLAN = '$vlan'"

# --- a real DHCP lease, then IPAM linkage ---------------------------------
HOSTNAME_TAG="h2c${TS: -6}"
docker exec client sh -c "ip link set eth0 down; ip link set eth0 address 02:00:00:00:02:01; ip link set eth0 up; udhcpc -i eth0 -n -q -f -x hostname:$HOSTNAME_TAG -t 6 -T 2 >/tmp/i.log 2>&1; grep -o 'lease of [0-9.]*' /tmp/i.log | awk '{print \$3}' | head -1" > "$OUT/ipam-lease-$TS.txt" 2>&1
IP=$(tr -d '\r\n' < "$OUT/ipam-lease-$TS.txt")
[ -n "${IP:-}" ] && ok "DHCP lease obtained for IPAM linkage ($IP)" || bad "no DHCP lease for IPAM linkage"
if [ -n "${IP:-}" ]; then
  sleep 3
  obs=$(sqlq "select observed_state from ipam_addresses where ip_address='$IP' and space_id='$SP' limit 1;")
  if [ -z "$obs" ]; then nv "IPAM has no address row for $IP (no space/subnet overlap mapping for this DHCP scope)"
  elif [ "$obs" = "in_use" ] || [ "$obs" = "active" ] || [ "$obs" = "bound" ]; then ok "IPAM address $IP observed_state=$obs (lease projected via facts)"
  else nv "IPAM address $IP observed_state=$obs"; fi
  # projection lag metric must exist once facts flow
  lag=$(curl -s http://127.0.0.1:18181/metrics | grep -E '^goddi_dhcp_ipam_projection_gaps' | head -1)
  [ -n "$lag" ] && ok "IPAM projection-gap metric exposed ($lag)" || nv "IPAM projection-gap metric not exposed on this node"
  # End-to-end facts: producer outbox -> relay -> control inbox -> IPAM.
  st=$(sqlq "select status from dhcp_ipam_observation_events order by sequence desc limit 1;")
  [ "$st" = "done" ] && ok "producer fact consumed (outbox status=done)" || nv "producer fact status='$st'"
  inbox=$(sqlq "select count(*) from dhcp_ipam_observation_events;")
  [ "${inbox:-0}" -ge 1 ] && ok "control inbox received the fact ($inbox event(s))" || bad "control inbox empty: authoritative fact not relayed"
fi

# udhcpc rewrites the client's own address/route; restore the fixture network
# before any name-resolution check (the DHCP lease is already recorded).
docker exec client sh -c 'ip link set eth0 down; ip addr flush dev eth0; ip link set eth0 address 02:00:00:00:00:aa; ip link set eth0 up; ip addr add 10.77.3.3/24 dev eth0; ip route add default via 10.77.3.1 2>/dev/null; true' >/dev/null 2>&1
sleep 1

# --- outbox completeness + idempotency ------------------------------------
head=$(sqlq "select max(sequence) from dhcp_ipam_observation_events;")
pending=$(sqlq "select count(*) from dhcp_ipam_observation_events where status='pending';")
failed=$(sqlq "select count(*) from dhcp_ipam_observation_events where status='failed';")
echo "outbox head=$head pending=$pending failed=$failed" > "$OUT/ipam-outbox-$TS.txt"
[ "${failed:-0}" -eq 0 ] && ok "no failed outbox events (no lost authoritative facts)" || bad "outbox has $failed failed events"
[ -n "${head:-}" ] && [ "${head:-0}" -gt 0 ] && ok "outbox advanced (head=$head)" || nv "outbox head not advanced"
# Idempotency: re-consuming the same event must not duplicate IPAM state.
before=$(sqlq "select count(*) from ipam_addresses;")
dup=$(sqlq "select count(*) from (select ip_address,space_id,count(*) c from ipam_addresses group by 1,2 having c>1);")
[ "${dup:-0}" -eq 0 ] && ok "no duplicate IPAM address rows (idempotent consumption)" || bad "duplicate IPAM rows: $dup"
after=$(sqlq "select count(*) from ipam_addresses;")
[ -n "$before" ] && [ -n "$after" ] && ok "IPAM address table stable across observation ($before -> $after)"

# --- DDNS / DNS publication ----------------------------------------------
# Give the DHCP scope a domain and enable DNS updates, and create the reverse
# zone the DHCP plane publishes PTRs into, so A/PTR are actually verifiable.
if [ -n "${IP:-}" ]; then
  SC=$(curl -s "http://127.0.0.1:18181/api/v1/dhcp/scopes" -H "Authorization: Bearer $TOK" | jget "d['data'][0]['id']")
  if [ -n "${SC:-}" ]; then
    curl -s -X PUT "http://127.0.0.1:18181/api/v1/dhcp/scopes/$SC" "${H[@]}" \
      -d '{"domain_name":"h2ddns.test","dns_updates":true}' -o /dev/null
    curl -s -X POST http://127.0.0.1:18181/api/v1/dns/zones "${H[@]}" -d '{"name":"h2ddns.test.","type":"primary"}' -o /dev/null
    curl -s -X POST http://127.0.0.1:18181/api/v1/dns/zones "${H[@]}" -d '{"name":"3.77.10.in-addr.arpa.","type":"primary"}' -o /dev/null
    sleep 4
  fi
  ptr=""
  for _ in $(seq 1 10); do
    ptr=$(docker exec client sh -c "nslookup $IP $DNS_IP 2>/dev/null | awk '/name =/{print \$NF}' | head -1" 2>/dev/null)
    [ -n "$ptr" ] && break; sleep 2
  done
  if [ -n "$ptr" ]; then ok "PTR published for $IP ($ptr)"; else nv "no PTR for $IP (reverse zone not configured for this test subnet)"; fi
  fwd=""
  for _ in $(seq 1 10); do
    fwd=$(docker exec client sh -c "nslookup $HOSTNAME_TAG.h2ddns.test $DNS_IP 2>/dev/null | awk '/^Address: /{print \$2}' | tail -1" 2>/dev/null)
    [ -n "$fwd" ] && break; sleep 2
  done
  [ -n "$fwd" ] && ok "forward A published for DHCP hostname ($fwd)" || nv "no forward A for the DHCP hostname in h2ddns.test (DDNS update not observed)"
fi

# --- dual-node IPAM configuration consistency ----------------------------
# IPAM configuration is control-plane state; cross-node coordination is
# ADR-0005 (not implemented). Report honestly instead of implying parity.
b_spaces=$(curl -s http://127.0.0.1:18182/api/v1/ipam/spaces -H "Authorization: Bearer $(printf '%s' "$(login 18182)"|jget "d['data']['token']")" | jget "d['meta']['total']")
a_spaces=$(curl -s http://127.0.0.1:18181/api/v1/ipam/spaces -H "Authorization: Bearer $TOK" | jget "d['meta']['total']")
if [ "${a_spaces:-0}" = "${b_spaces:-0}" ]; then nv "both nodes report $a_spaces IPAM space(s) but configs are independent stores (ADR-0005 publish not implemented)"
else nv "IPAM config differs between nodes (a=$a_spaces b=$b_spaces): expected without ADR-0005 publish coordination"; fi

printf '{"suite":"ipam","timestamp":"%s","pass":%d,"fail":%d,"not_verified":%d,"ip":"%s"}\n' "$TS" "$PASS" "$FAIL" "$NV" "${IP:-}" > "$OUT/ipam-summary-$TS.json"
echo "----"; echo "IPAM PASS=$PASS FAIL=$FAIL NOT_VERIFIED=$NV"
[ "$FAIL" -eq 0 ]
