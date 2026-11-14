#!/usr/bin/env bash
# Seeds a two-node demo with representative test data on node-a (the author).
# Configuration then replicates to node-b via ADR-0013 (when the demo overlay is
# used), and dynamic leases are mirrored by the DHCP HA channel.
set -uo pipefail
cd "$(dirname "$0")/.."
API=http://127.0.0.1:18181
jget(){ python3 -c "import sys,json;d=json.load(sys.stdin);print($1)" 2>/dev/null; }
R=$(curl -s -X POST "$API/api/v1/auth/login" -H 'Content-Type: application/json' -d '{"username":"admin","password":"AdminPass-0123456789"}')
T=$(printf '%s' "$R"|jget "d['data']['token']"); C=$(printf '%s' "$R"|jget "d['data']['csrf_token']")
H=(-H "Authorization: Bearer $T" -H "X-CSRF-Token: $C" -H 'Content-Type: application/json')
post(){ curl -s -X POST "$API$1" "${H[@]}" -d "$2"; }
put(){ curl -s -X PUT "$API$1" "${H[@]}" -d "$2"; }

mkzone(){ local nm=$1 ty=${2:-primary}; post /api/v1/dns/zones "{\"name\":\"$nm\",\"type\":\"$ty\"}" | jget "d['data']['id']"; }
mkrec(){ post "/api/v1/dns/zones/$1/records" "$2" >/dev/null; }

echo "### DNS zones + records"
Z=$(mkzone "demo.test.")
[ -n "${Z:-}" ] && echo "  zone demo.test. = $Z" || echo "  zone demo.test. create failed"
mkrec "$Z" '{"name":"@","type":"A","ttl":300,"value":"10.77.3.10"}'
mkrec "$Z" '{"name":"www","type":"A","ttl":300,"value":"10.77.3.11"}'
mkrec "$Z" '{"name":"api","type":"A","ttl":300,"value":"10.77.3.12"}'
mkrec "$Z" '{"name":"mail","type":"A","ttl":300,"value":"10.77.3.13"}'
mkrec "$Z" '{"name":"ftp","type":"CNAME","ttl":300,"value":"www.demo.test."}'
mkrec "$Z" '{"name":"@","type":"TXT","ttl":300,"value":"v=spf1 mx -all"}'
mkrec "$Z" '{"name":"@","type":"MX","ttl":300,"value":"mail.demo.test.","priority":10}'
mkrec "$Z" '{"name":"_sip._tcp","type":"SRV","ttl":300,"value":"10.77.3.12","priority":10,"weight":60,"port":5060}'
RZ=$(mkzone "3.77.10.in-addr.arpa.")
[ -n "${RZ:-}" ] && echo "  zone reverse = $RZ" || echo "  reverse zone create failed"
mkrec "$RZ" '{"name":"10","type":"PTR","ttl":300,"value":"www.demo.test."}'
mkrec "$RZ" '{"name":"11","type":"PTR","ttl":300,"value":"www.demo.test."}'

echo "### DHCP scope + reservation"
SC=$(post /api/v1/dhcp/scopes '{"name":"demo-lan","subnet":"10.77.3.0/24","start_ip":"10.77.3.150","end_ip":"10.77.3.200","lease_time":600,"domain_name":"demo.test","dns_updates":true,"router":"10.77.3.1","dns_servers":"10.77.3.4"}' | jget "d['data']['id']")
[ -n "${SC:-}" ] && echo "  scope demo-lan = $SC" || echo "  scope create failed"
if [ -n "${SC:-}" ]; then
  post /api/v1/dhcp/reservations "{\"scope_id\":\"$SC\",\"ip_address\":\"10.77.3.160\",\"mac_address\":\"02:00:00:00:00:aa\",\"hostname\":\"demo-printer\"}" | jget "d['code']" | { read c; echo "  reservation .160 -> code=$c"; }
  post /api/v1/dhcp/options "{\"scope_id\":\"$SC\",\"code\":66,\"value\":\"boot.demo.test\",\"priority\":0}" | jget "d['code']" | { read c; echo "  option 66 -> code=$c"; }
fi

echo "### IPAM space + subnet"
SP=$(post /api/v1/ipam/spaces '{"name":"demo-space","description":"demo addressing"}' | jget "d['data']['id']")
[ -n "${SP:-}" ] && echo "  space demo-space = $SP" || echo "  space create failed"
if [ -n "${SP:-}" ]; then
  post /api/v1/ipam/subnets "{\"space_id\":\"$SP\",\"name\":\"demo-subnet\",\"cidr\":\"10.77.3.0/24\",\"vlan_id\":130,\"location\":\"h2\"}" | jget "d['code']" | { read c; echo "  subnet create -> code=$c"; }
fi

echo "### dynamic leases (real udhcpc from the client container)"
setmac(){ docker exec client sh -c "ip link set eth0 down; ip link set eth0 address $1; ip link set eth0 up" >/dev/null 2>&1; }
udhcpc(){ docker exec client sh -c 'rm -f /tmp/u.log; udhcpc -i eth0 -n -q -f -x hostname:h2c -t 6 -T 2 >/tmp/u.log 2>&1; grep -o "lease of [0-9.]*" /tmp/u.log | head -1'; }
for m in 02:00:00:00:00:01 02:00:00:00:00:02 02:00:00:00:00:03; do
  setmac "$m"; L=$(udhcpc); echo "  lease for $m: ${L:-none}"
done
sleep 1
echo "### counts"
echo -n "  zones: "; curl -s "$API/api/v1/dns/zones" -H "Authorization: Bearer $T" | jget "len(d['data'])"
echo -n "  scopes: "; curl -s "$API/api/v1/dhcp/scopes" -H "Authorization: Bearer $T" | jget "len(d['data'])"
echo -n "  leases: "; curl -s "$API/api/v1/dhcp/leases" -H "Authorization: Bearer $T" | jget "d['meta']['total']"
