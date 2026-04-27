#!/usr/bin/env bash
# Generate a short-lived self-signed cert for masque-server LISTEN_TLS_CERT / LISTEN_TLS_KEY (dev only).
set -euo pipefail
OUT="${1:-.}"
mkdir -p "${OUT}"
openssl req -x509 -newkey rsa:2048 \
	-keyout "${OUT}/masque-listen.key" \
	-out "${OUT}/masque-listen.crt" \
	-days 30 -nodes \
	-subj "/CN=localhost"
echo "Wrote ${OUT}/masque-listen.crt and ${OUT}/masque-listen.key"
echo "Run: LISTEN_TLS_CERT=${OUT}/masque-listen.crt LISTEN_TLS_KEY=${OUT}/masque-listen.key masque-server"
echo "Set MASQUE_SERVER_URL=https://127.0.0.1:8443 (or matching host) on the control plane; curl -k for local checks."
