#!/usr/bin/env bash
# Generate self-signed TLS certificates for MQTT broker.
# Usage: ./scripts/gen-mqtt-tls.sh [output_dir] [cn]
#   output_dir  directory for generated files (default: ./certs/mqtt)
#   cn          Common Name for server cert (default: iot.local)
set -euo pipefail

OUT="${1:-./certs/mqtt}"
CN="${2:-iot.local}"
DAYS_CA=3650
DAYS_CERT=365
CURVE=prime256v1

mkdir -p "$OUT"

echo "==> Generating CA (valid ${DAYS_CA} days)..."
openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:"$CURVE" \
  -keyout "$OUT/ca-key.pem" -out "$OUT/ca.pem" \
  -days "$DAYS_CA" -nodes \
  -subj "/CN=Cogitor IoT CA"

echo "==> Generating server certificate (CN=${CN}, valid ${DAYS_CERT} days)..."
openssl req -newkey ec -pkeyopt ec_paramgen_curve:"$CURVE" \
  -keyout "$OUT/key.pem" -out "$OUT/server.csr" -nodes \
  -subj "/CN=${CN}"

# SAN extension for IP/DNS (common for IoT deployments)
cat > "$OUT/server-ext.cnf" <<EOF
[v3_req]
subjectAltName = @alt_names
[alt_names]
DNS.1 = ${CN}
DNS.2 = localhost
IP.1 = 127.0.0.1
EOF

openssl x509 -req -in "$OUT/server.csr" \
  -CA "$OUT/ca.pem" -CAkey "$OUT/ca-key.pem" -CAcreateserial \
  -out "$OUT/cert.pem" -days "$DAYS_CERT" \
  -extfile "$OUT/server-ext.cnf" -extensions v3_req

rm -f "$OUT/server.csr" "$OUT/server-ext.cnf"

echo "==> Generating client certificate (for mutual TLS, valid ${DAYS_CERT} days)..."
openssl req -newkey ec -pkeyopt ec_paramgen_curve:"$CURVE" \
  -keyout "$OUT/client-key.pem" -out "$OUT/client.csr" -nodes \
  -subj "/CN=device-client"

openssl x509 -req -in "$OUT/client.csr" \
  -CA "$OUT/ca.pem" -CAkey "$OUT/ca-key.pem" -CAcreateserial \
  -out "$OUT/client.pem" -days "$DAYS_CERT"

rm -f "$OUT/client.csr"

# Remove serial file
rm -f "$OUT/ca.srl"

echo ""
echo "==> Certificates generated in ${OUT}/"
echo ""
echo "Files:"
echo "  ca.pem          CA certificate (give to devices for server verification)"
echo "  ca-key.pem      CA private key (keep secret)"
echo "  cert.pem        Server certificate (set as MQTT_TLS_CERT)"
echo "  key.pem         Server private key (set as MQTT_TLS_KEY)"
echo "  client.pem      Client certificate (for mutual TLS)"
echo "  client-key.pem  Client private key (for mutual TLS)"
echo ""
echo "Enable TLS:"
echo "  export MQTT_TLS_CERT=${OUT}/cert.pem"
echo "  export MQTT_TLS_KEY=${OUT}/key.pem"
echo ""
echo "Enable mutual TLS (optional):"
echo "  export MQTT_TLS_CA=${OUT}/ca.pem"
