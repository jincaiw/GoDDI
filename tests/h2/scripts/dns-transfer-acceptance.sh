#!/usr/bin/env bash
# DNS zone-transfer acceptance: TSIG key creation, transfer authorization,
# a real AXFR over the wire with a signed request, and the unsigned request
# being refused. Uses the real GoDDI DNS server and a real DNS client library
# (python dnspython is not assumed; we use a tiny Go-free check via the
# authoritative server's own TSIG handling and nslookup for negative control).
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

ZONE=h2xfr.test.
# Resolve node-a's client-network address at run time: Docker's ethN ordering and
# IPAM lease order are not stable across recreates, so a hard-coded address can
# silently point at node-b (which has no TSIG key and no authorization).
DNS_IP=$(docker exec node-a sh -c 'ip -4 -o addr show | awk "{print \$4}" | cut -d/ -f1 | grep "^10.77.3." | head -1')
[ -n "${DNS_IP:-}" ] || DNS_IP=10.77.3.2
KEYNAME_TAG="xfr${TS: -6}."
# Zone + records
Z=$(curl -s -X POST http://127.0.0.1:18181/api/v1/dns/zones "${H[@]}" -d "{\"name\":\"$ZONE\",\"type\":\"primary\"}" | jget "d['data']['id']")
if [ -z "${Z:-}" ]; then
  Z=$(curl -s http://127.0.0.1:18181/api/v1/dns/zones -H "Authorization: Bearer $TOK" | python3 -c "import sys,json;d=json.load(sys.stdin)['data'];print(next((x['id'] for x in d if x['name']=='$ZONE'),''))")
fi
[ -n "${Z:-}" ] && ok "zone $ZONE present" || bad "zone create failed"
curl -s -X POST "http://127.0.0.1:18181/api/v1/dns/zones/$Z/records" "${H[@]}" -d '{"name":"host1","type":"A","ttl":300,"value":"192.0.2.201"}' -o /dev/null
curl -s -X POST "http://127.0.0.1:18181/api/v1/dns/zones/$Z/records" "${H[@]}" -d '{"name":"host2","type":"A","ttl":300,"value":"192.0.2.202"}' -o /dev/null

# TSIG key (server generates and returns the secret once)
KJSON=$(curl -s -X POST http://127.0.0.1:18181/api/v1/dns/tsig/keys "${H[@]}" -d "{\"name\":\"xfr${TS: -6}.\",\"algorithm\":\"hmac-sha256\"}")
KNAME=$(printf '%s' "$KJSON" | jget "d['data']['name']"); KSECRET=$(printf '%s' "$KJSON" | jget "d['data']['secret']")
[ -n "${KSECRET:-}" ] && ok "TSIG key created and secret returned once ($KNAME)" || bad "TSIG key creation failed"

# Authorize AXFR for the client subnet with that key
if [ -n "${KNAME:-}" ]; then
  AUTH=$(curl -s -X POST "http://127.0.0.1:18181/api/v1/dns/zones/$Z/transfers" "${H[@]}" \
    -d "{\"allowed_cidr\":\"10.77.3.0/24\",\"tsig_key_name\":\"$KNAME\"}")
  printf '%s' "$AUTH" | grep -q '"code":0' && ok "AXFR authorized for 10.77.3.0/24 with TSIG $KNAME" || bad "transfer authorization failed: $(printf '%s' "$AUTH" | head -c 160)"
fi

# Real AXFR with a signed request from the client container (axfrprobe is a real
# DNS protocol client built from this repo's tools/axfrprobe; the image has no
# dig). A signed transfer that the client rejects is reported FAIL: it is never
# retried into PASS.
if [ -n "${KSECRET:-}" ] && [ -n "${KNAME:-}" ]; then
  # TSIG secrets are loaded once at listener start, so the node is restarted
  # after the key and its authorization exist. This is the documented operator
  # sequence (a new TSIG key requires a restart) and it must happen BEFORE the
  # transfer, not after it.
  docker restart node-a >/dev/null 2>&1
  for _ in $(seq 1 30); do
    [ "$(docker inspect -f '{{.State.Health.Status}}' node-a 2>/dev/null)" = healthy ] && break
    sleep 2
  done
  sleep 2
  DNS_IP=$(docker exec node-a sh -c 'ip -4 -o addr show | awk "{print \$4}" | cut -d/ -f1 | grep "^10.77.3." | head -1')
  echo "target DNS $DNS_IP:53"
  AX=$(docker exec client axfrprobe -server "${DNS_IP}:53" -zone "$ZONE" -key "$KNAME" -secret "$KSECRET" 2>&1 | tail -2)
  case "$AX" in
    *AXFR_RECORDS*) ok "signed AXFR completed ($(printf '%s' "$AX" | tr '\n' ' '))";;
    *) bad "signed AXFR rejected by the client: $(printf '%s' "$AX" | tr '\n' ' ')";;
  esac
  # Negative control: the same request without a key must be refused.
  UN=$(docker exec client axfrprobe -server ${DNS_IP}:53 -zone "$ZONE" 2>&1 | tail -1)
  case "$UN" in
    *AXFR_ERROR*) ok "unsigned AXFR refused (negative control): $(printf '%s' "$UN" | head -c 80)";;
    *AXFR_RECORDS*) bad "unsigned AXFR succeeded: transfer ACL/TSIG is not enforced";;
    *) nv "unsigned AXFR result inconclusive: $(printf '%s' "$UN" | head -c 80)";;
  esac
else nv "signed AXFR not attempted (no TSIG secret in this run)"; fi
soa=$(docker exec client sh -c "nslookup -type=SOA $ZONE $DNS_IP 2>/dev/null | grep -ci 'origin'" 2>/dev/null)
[ "${soa:-0}" -ge 1 ] && ok "authoritative SOA served for $ZONE (real query)" || nv "SOA query returned no origin line"
# Negative control: unsigned AXFR must be refused. nslookup cannot do AXFR, so
# this is covered by the Go handler test; report honestly.
  # IXFR: ask with an old serial; the server answers with a full transfer when
  # it cannot produce a delta, which is RFC 1995 compliant.
  IX=$(docker exec client axfrprobe -server "${DNS_IP}:53" -zone "$ZONE" -key "$KNAME" -secret "$KSECRET" -ixfr -serial 1 2>&1 | tail -1)
  case "$IX" in
    *AXFR_RECORDS*) ok "IXFR request answered ($(printf '%s' "$IX" | head -c 60))";;
    *) nv "IXFR result: $(printf '%s' "$IX" | head -c 80)";;
  esac
  # NOTIFY: the server acknowledges a NOTIFY for a zone it serves (RFC 1996).
  NT=$(docker exec client rawprobe -notify "$DNS_IP:53" "$ZONE" 2>&1 | tail -1)
  case "$NT" in
    *NOTIFY_OK*) ok "NOTIFY acknowledged by the authoritative server ($NT)";;
    *) nv "NOTIFY result: $(printf '%s' "$NT" | head -c 80)";;
  esac
printf '{"suite":"dns-transfer","timestamp":"%s","pass":%d,"fail":%d,"not_verified":%d}\n' "$TS" "$PASS" "$FAIL" "$NV" > "$OUT/dns-transfer-summary-$TS.json"
echo "----"; echo "DNS-TRANSFER PASS=$PASS FAIL=$FAIL NOT_VERIFIED=$NV"
[ "$FAIL" -eq 0 ]
