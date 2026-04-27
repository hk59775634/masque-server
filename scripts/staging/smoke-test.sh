#!/usr/bin/env bash
set -euo pipefail

CONTROL_PLANE_URL="${CONTROL_PLANE_URL:-https://www.afbuyers.com}"
MASQUE_SERVER_URL="${MASQUE_SERVER_URL:-http://127.0.0.1:8443}"
PROMETHEUS_URL="${PROMETHEUS_URL:-http://127.0.0.1:9090}"
SKIP_PROMETHEUS="${SKIP_PROMETHEUS:-0}"
LOKI_URL="${LOKI_URL:-http://127.0.0.1:3100}"
SKIP_LOKI="${SKIP_LOKI:-0}"
MASQUE_QUIC_URL="${MASQUE_QUIC_URL:-}"
SKIP_MASQUE_QUIC="${SKIP_MASQUE_QUIC:-1}"

echo "[smoke] control plane: ${CONTROL_PLANE_URL}"
echo "[smoke] masque server: ${MASQUE_SERVER_URL}"
echo "[smoke] prometheus: ${PROMETHEUS_URL} (SKIP_PROMETHEUS=${SKIP_PROMETHEUS})"
echo "[smoke] loki: ${LOKI_URL} (SKIP_LOKI=${SKIP_LOKI})"
echo "[smoke] masque quic (HTTP/3): ${MASQUE_QUIC_URL:-<unset>} (SKIP_MASQUE_QUIC=${SKIP_MASQUE_QUIC})"

echo "[smoke] check control-plane reachable..."
if curl -fsS "${CONTROL_PLANE_URL}/api/v1/health" >/dev/null 2>&1; then
  echo "[smoke] control-plane OK (/api/v1/health)"
else
  echo "[smoke] control-plane fallback (/login)..."
  curl -fsS "${CONTROL_PLANE_URL}/login" >/dev/null
fi

echo "[smoke] check masque health..."
curl -fsS "${MASQUE_SERVER_URL}/healthz" >/dev/null

echo "[smoke] check masque capabilities..."
curl -fsS "${MASQUE_SERVER_URL}/v1/masque/capabilities" >/dev/null

if [[ "${SKIP_MASQUE_QUIC}" == "1" || -z "${MASQUE_QUIC_URL}" ]]; then
  echo "[smoke] skip masque HTTP/3 (set MASQUE_QUIC_URL=https://127.0.0.1:8444 and SKIP_MASQUE_QUIC=0; needs curl with HTTP/3)"
else
  if ! curl --help all 2>/dev/null | grep -q 'http3'; then
    echo "[smoke] WARN skip masque HTTP/3: curl on this host has no HTTP/3 support (install curl with nghttp3/quiche, or leave SKIP_MASQUE_QUIC=1)"
  else
    echo "[smoke] check masque HTTP/3 healthz..."
    curl -fsS --http3-only --insecure "${MASQUE_QUIC_URL}/healthz" >/dev/null
    echo "[smoke] check masque HTTP/3 capabilities..."
    curl -fsS --http3-only --insecure "${MASQUE_QUIC_URL}/v1/masque/capabilities" >/dev/null
  fi
fi

if [[ "${SKIP_PROMETHEUS}" == "1" ]]; then
  echo "[smoke] skip prometheus (SKIP_PROMETHEUS=1)"
else
  echo "[smoke] check prometheus api..."
  curl -fsS "${PROMETHEUS_URL}/api/v1/targets" >/dev/null
fi

if [[ "${SKIP_LOKI}" == "1" ]]; then
  echo "[smoke] skip loki (SKIP_LOKI=1)"
else
  echo "[smoke] check loki ready..."
  code="$(curl -sS -o /dev/null -w "%{http_code}" "${LOKI_URL}/ready" || echo 000)"
  if [[ "${code}" == "200" ]]; then
    echo "[smoke] loki /ready OK"
  elif [[ "${code}" == "503" ]]; then
    echo "[smoke] WARN loki /ready -> 503 (ring not ready yet; retry or SKIP_LOKI=1)"
  else
    echo "[smoke] FAIL loki /ready -> ${code}"
    exit 1
  fi
fi

echo "[smoke] all checks passed."
