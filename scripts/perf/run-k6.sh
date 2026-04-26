#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

echo "[k6] starting connect load test..."
echo "[k6] MASQUE_SERVER_URL=${MASQUE_SERVER_URL:-http://127.0.0.1:8443}"
echo "[k6] VUS=${VUS:-20} DURATION=${DURATION:-60s}"

docker run --rm \
  --network host \
  -e MASQUE_SERVER_URL="${MASQUE_SERVER_URL:-http://127.0.0.1:8443}" \
  -e DEVICE_TOKEN="${DEVICE_TOKEN:-}" \
  -e FINGERPRINT="${FINGERPRINT:-k6-fingerprint}" \
  -e DEST_IP="${DEST_IP:-1.1.1.1}" \
  -e DEST_PORT="${DEST_PORT:-443}" \
  -e PROTOCOL="${PROTOCOL:-tcp}" \
  -e VUS="${VUS:-20}" \
  -e DURATION="${DURATION:-60s}" \
  -v "${ROOT_DIR}/scripts/perf:/work" \
  grafana/k6 run /work/k6-connect.js
