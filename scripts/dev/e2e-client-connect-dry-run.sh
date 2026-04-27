#!/usr/bin/env bash
# End-to-end: control-plane (user + code + activate) → masque POST /connect → client connect -dry-run.
# Prerequisites:
#   - jq, curl
#   - control-plane: php artisan serve --host=127.0.0.1 --port=8000 (or set CONTROL_PLANE_URL)
#   - masque-server: CONTROL_PLANE_URL matching the same host go run ./cmd/server (or set MASQUE_SERVER_URL for client side)
# Does not touch ~/.masque-client.json (uses temp paths via MASQUE_CLIENT_CONFIG).
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
CP="${CONTROL_PLANE_URL:-http://127.0.0.1:8000}"
MASQUE_CLIENT_BIN="${MASQUE_CLIENT_BIN:-}"
CP="${CP%/}"
MASQUE="${MASQUE_SERVER_URL:-http://127.0.0.1:8443}"
MASQUE="${MASQUE%/}"

WORKDIR="$(mktemp -d)"
trap 'rm -rf "${WORKDIR}"' EXIT
export MASQUE_CLIENT_CONFIG="${WORKDIR}/client.json"
export MASQUE_CLIENT_STATE="${WORKDIR}/state.json"

if ! command -v jq >/dev/null 2>&1; then
	echo "error: jq is required"
	exit 1
fi

echo "[e2e] control plane: ${CP}"
echo "[e2e] masque server (expected in activate response): ${MASQUE}"

curl -fsS "${CP}/api/v1/health" >/dev/null
curl -fsS "${MASQUE}/healthz" >/dev/null

EMAIL="e2e-$(date +%s)@example.test"
FP="fp-e2e-$(date +%s)"

USER_JSON="$(curl -fsS -X POST "${CP}/api/v1/users" \
	-H 'Content-Type: application/json' \
	-d "{\"name\":\"E2E\",\"email\":\"${EMAIL}\",\"password\":\"password123\"}")"
USER_ID="$(echo "${USER_JSON}" | jq -r '.user.id')"
echo "[e2e] created user id=${USER_ID}"

CODE_JSON="$(curl -fsS -X POST "${CP}/api/v1/devices/activation-code" \
	-H 'Content-Type: application/json' \
	-d "{\"user_id\":${USER_ID},\"device_name\":\"e2e-cli\",\"fingerprint\":\"${FP}\"}")"
RAW_CODE="$(echo "${CODE_JSON}" | jq -r '.activation_code')"
echo "[e2e] activation code issued (redacted)"

run_client() {
	if [[ -n "${MASQUE_CLIENT_BIN}" ]]; then
		"${MASQUE_CLIENT_BIN}" "$@"
	else
		( cd "${ROOT}/linux-client" && go run ./cmd/client "$@" )
	fi
}

run_client activate \
	-control-plane "${CP}" \
	-fingerprint "${FP}" \
	-code "${RAW_CODE}"

saved="$(jq -r '.masque_server_url' "${MASQUE_CLIENT_CONFIG}")"
if [[ "${saved}" != "${MASQUE}" ]]; then
	echo "[e2e] WARN: saved masque_server_url=${saved} (expected ${MASQUE}). Set MASQUE_SERVER_URL in control-plane .env to match this masque, then re-run activate."
fi

run_client connect -dry-run

echo "[e2e] OK — full data path (ip route + resolv.conf) with: sudo go run ./cmd/client connect [-check]"
