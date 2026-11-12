#!/usr/bin/env bash
# Generate a private CA and one node cert per node (mTLS). Idempotent: keeps
# existing certs unless FORCE=1. SANs pin the replication-network IPs so the
# peer channel is verified by identity, not by network position.
set -euo pipefail
cd "$(dirname "$0")/.."
C="$PWD/certs"; mkdir -p "$C"
if [ "${FORCE:-0}" != "1" ] && [ -f "$C/ca.crt" ] && [ -f "$C/h2-node-a.crt" ] && [ -f "$C/h2-node-b.crt" ]; then
  echo "certs already present ($C)"; exit 0
fi
command -v openssl >/dev/null || { echo "openssl is required to generate certs" >&2; exit 2; }
rm -f "$C"/*.crt "$C"/*.key "$C"/*.csr "$C"/*.srl "$C"/*.ext
openssl genrsa -out "$C/ca.key" 2048 >/dev/null 2>&1
openssl req -x509 -new -key "$C/ca.key" -sha256 -days 3650 -subj "/CN=goddi-h2-ca" -out "$C/ca.crt" >/dev/null 2>&1
mk_node(){ name="$1"; ip="$2";
  openssl genrsa -out "$C/$name.key" 2048 >/dev/null 2>&1
  openssl req -new -key "$C/$name.key" -subj "/CN=$name" -out "$C/$name.csr" >/dev/null 2>&1
  cat > "$C/$name.ext" <<EXT
subjectAltName=DNS:$name,IP:$ip,IP:127.0.0.1
extendedKeyUsage=serverAuth,clientAuth
keyUsage=digitalSignature,keyEncipherment
EXT
  openssl x509 -req -in "$C/$name.csr" -CA "$C/ca.crt" -CAkey "$C/ca.key" -CAcreateserial \
    -days 3650 -sha256 -extfile "$C/$name.ext" -out "$C/$name.crt" >/dev/null 2>&1
}
mk_node h2-node-a 10.77.2.11
mk_node h2-node-b 10.77.2.12
chmod 644 "$C"/*.crt; chmod 600 "$C"/*.key
echo "certs written to $C"
