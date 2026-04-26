#!/usr/bin/env bash
set -euo pipefail

CONTROL_PLANE_URL="${CONTROL_PLANE_URL:-https://www.afbuyers.com}"
MASQUE_SERVER_URL="${MASQUE_SERVER_URL:-http://127.0.0.1:8443}"

echo "[smoke] control plane: ${CONTROL_PLANE_URL}"
echo "[smoke] masque server: ${MASQUE_SERVER_URL}"

echo "[smoke] check control-plane reachable..."
if curl -fsS "${CONTROL_PLANE_URL}/api/v1/health" >/dev/null 2>&1; then
  echo "[smoke] control-plane OK (/api/v1/health)"
else
  echo "[smoke] control-plane fallback (/login)..."
  curl -fsS "${CONTROL_PLANE_URL}/login" >/dev/null
fi

echo "[smoke] check masque health..."
curl -fsS "${MASQUE_SERVER_URL}/healthz" >/dev/null

echo "[smoke] check prometheus scrape target..."
curl -fsS "http://127.0.0.1:9090/api/v1/targets" >/dev/null

echo "[smoke] all checks passed."
