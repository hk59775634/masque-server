#!/usr/bin/env bash
# One-shot: email + password → fetch activation code → masque-client activate → connect.
# Requires: curl, python3, masque-client (PATH or MASQUE_CLIENT).
#
# Env (optional):
#   CONTROL_PLANE_URL   default https://www.afbuyers.com
#   MASQUE_CLIENT       path to masque-client binary (default: masque-client from PATH)
#   CONNECT_MODE        dry-run | real  (default: dry-run; real uses sudo for routes/DNS)
set -euo pipefail

CONTROL_PLANE_URL="${CONTROL_PLANE_URL:-https://www.afbuyers.com}"
CP="${CONTROL_PLANE_URL%/}"
MASQUE_CLIENT="${MASQUE_CLIENT:-masque-client}"
CONNECT_MODE="${CONNECT_MODE:-dry-run}"

CONFIG_DEFAULT="${HOME}/.masque-client.json"
if [[ -n "${MASQUE_CLIENT_CONFIG:-}" ]]; then
	CONFIG_FILE="${MASQUE_CLIENT_CONFIG}"
else
	CONFIG_FILE="${CONFIG_DEFAULT}"
fi

FP_DIR="${XDG_CONFIG_HOME:-${HOME}/.config}/masque-linux-client"
FP_FILE="${FP_DIR}/device-fingerprint"

die() { echo "error: $*" >&2; exit 1; }

command -v curl >/dev/null 2>&1 || die "curl not found"
command -v python3 >/dev/null 2>&1 || die "python3 not found"
if [[ "${MASQUE_CLIENT}" != *'/'* ]] && ! command -v "${MASQUE_CLIENT}" >/dev/null 2>&1; then
	die "masque-client not found (set MASQUE_CLIENT=/path/to/masque-client)"
fi

mkdir -p "${FP_DIR}"
if [[ -f "${FP_FILE}" ]]; then
	FINGERPRINT="$(tr -d '\r\n' < "${FP_FILE}")"
	[[ -n "${FINGERPRINT}" ]] || die "empty ${FP_FILE}"
else
	FINGERPRINT="fp-$(hostname)-$(python3 -c 'import secrets; print(secrets.token_hex(8))')"
	printf '%s\n' "${FINGERPRINT}" > "${FP_FILE}"
	chmod 600 "${FP_FILE}"
	echo "[quick] wrote new device fingerprint to ${FP_FILE}"
fi

DEVICE_NAME="${DEVICE_NAME:-$(hostname)-quick}"

need_activate=1
if [[ -f "${CONFIG_FILE}" ]] && python3 -c "import json,sys; c=json.load(open(sys.argv[1])); sys.exit(0 if c.get('device_token') else 1)" "${CONFIG_FILE}" 2>/dev/null; then
	need_activate=0
	echo "[quick] using saved device config at ${CONFIG_FILE} (remove file to log in again with email/password)"
fi

if [[ "${need_activate}" == "1" ]]; then
	read -r -p "Control-plane email: " EMAIL
	[[ -n "${EMAIL// }" ]] || die "email required"
	read -r -s -p "Password: " PASSWORD
	echo
	[[ -n "${PASSWORD}" ]] || die "password required"

	TMP_BODY="$(mktemp)"
	cleanup() { rm -f "${TMP_BODY}" /tmp/masque-quick-code.json 2>/dev/null || true; }
	trap cleanup EXIT
	python3 -c '
import json, os, sys
email, password, fp, dn = sys.argv[1:5]
print(json.dumps({"email": email, "password": password, "fingerprint": fp, "device_name": dn}))
' "${EMAIL}" "${PASSWORD}" "${FINGERPRINT}" "${DEVICE_NAME}" > "${TMP_BODY}"

	URL="${CP}/api/v1/devices/activation-code-with-credentials"
	HTTP_CODE="$(curl -sS -o /tmp/masque-quick-code.json -w '%{http_code}' \
		-X POST "${URL}" \
		-H 'Content-Type: application/json' \
		--data-binary "@${TMP_BODY}")" || true

	if [[ "${HTTP_CODE}" != "201" ]]; then
		echo "error: ${URL} returned HTTP ${HTTP_CODE}" >&2
		cat /tmp/masque-quick-code.json >&2 || true
		exit 1
	fi

	RAW_CODE="$(python3 -c 'import json; print(json.load(open("/tmp/masque-quick-code.json"))["activation_code"])')"
	echo "[quick] activation code received; activating..."
	"${MASQUE_CLIENT}" activate \
		-control-plane "${CP}" \
		-fingerprint "${FINGERPRINT}" \
		-code "${RAW_CODE}" \
		-verify
fi

case "${CONNECT_MODE}" in
	dry-run)
		echo "[quick] connect (dry-run, no root)..."
		"${MASQUE_CLIENT}" connect -dry-run
		echo "[quick] done. For real VPN routes/DNS: CONNECT_MODE=real $0"
		;;
	real)
		echo "[quick] connect (real; requires sudo)..."
		sudo -E env "PATH=${PATH}" "${MASQUE_CLIENT}" connect -check
		echo "[quick] done."
		;;
	*)
		die "CONNECT_MODE must be dry-run or real (got ${CONNECT_MODE})"
		;;
esac
