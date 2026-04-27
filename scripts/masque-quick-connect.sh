#!/usr/bin/env bash
# Default flow (Linux): prompt control-plane URL + account → activate → sudo connect-ip-tun
# (creates tun0 by default, split IPv4 default routes, capsule routes, DNS from config or 1.1.1.1,8.8.8.8).
#
# Requires: curl, python3, masque-client (Linux build with connect-ip-tun), sudo, /dev/net/tun.
#
# Env (optional):
#   CONTROL_PLANE_URL     skip URL prompt when preset
#   MASQUE_EMAIL / MASQUE_PASSWORD   non-interactive activate (with need for config missing)
#   MASQUE_CLIENT         path to masque-client (default: masque-client in PATH)
#   TUN_NAME              default tun0
#   MASQUE_TUN_DNS        comma DNS e.g. 1.1.1.1,8.8.8.8 (else from ~/.masque-client.json dns[], else 1.1.1.1,8.8.8.8)
#   CONNECT_IP_UDP        QUIC UDP host:port (e.g. www.afbuyers.com:8444). If unset, script infers
#                         host from masque_server_url in ~/.masque-client.json + AUTO_CONNECT_IP_UDP_PORT (default 8444).
#   AUTO_CONNECT_IP_UDP   default 1; set 0 to skip inference (then you must set CONNECT_IP_UDP or enable QUIC on masque).
#   AUTO_CONNECT_IP_UDP_PORT  default 8444 (common alongside HTTP :8443)
#   MASQUE_SERVER_URL / DEFAULT_PUBLIC_MASQUE / SKIP_MASQUE_CONFIG_FIX   (fix loopback masque in saved JSON)
#   LEGACY_CONNECT=1      use old HTTP connect only (CONNECT_MODE=dry-run|real), no TUN
set -euo pipefail

die() { echo "error: $*" >&2; exit 1; }

if [[ "$(uname -s)" != "Linux" ]]; then
	die "this script targets Linux (connect-ip-tun)"
fi

DEFAULT_CP="${DEFAULT_CP:-https://www.afbuyers.com}"
if [[ -z "${CONTROL_PLANE_URL:-}" ]]; then
	if [[ -t 0 ]]; then
		read -r -p "Control plane URL [${DEFAULT_CP}]: " _cp_in || true
		CONTROL_PLANE_URL="${_cp_in:-$DEFAULT_CP}"
	else
		CONTROL_PLANE_URL="${DEFAULT_CP}"
	fi
fi
CP="${CONTROL_PLANE_URL%/}"

MASQUE_CLIENT="${MASQUE_CLIENT:-masque-client}"
TUN_NAME="${TUN_NAME:-tun0}"
DEFAULT_PUBLIC_MASQUE="${DEFAULT_PUBLIC_MASQUE:-http://www.afbuyers.com:8443}"
LEGACY_CONNECT="${LEGACY_CONNECT:-0}"
CONNECT_MODE="${CONNECT_MODE:-dry-run}"

CONFIG_DEFAULT="${HOME}/.masque-client.json"
if [[ -n "${MASQUE_CLIENT_CONFIG:-}" ]]; then
	CONFIG_FILE="${MASQUE_CLIENT_CONFIG}"
else
	CONFIG_FILE="${CONFIG_DEFAULT}"
fi

FP_DIR="${XDG_CONFIG_HOME:-${HOME}/.config}/masque-linux-client"
FP_FILE="${FP_DIR}/device-fingerprint"

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
	echo "[quick] using saved device config at ${CONFIG_FILE} (rm that file to log in again)"
fi

if [[ "${need_activate}" == "1" ]]; then
	EMAIL="${MASQUE_EMAIL:-}"
	PASSWORD="${MASQUE_PASSWORD:-}"
	if [[ -z "${EMAIL}" ]] && [[ -t 0 ]]; then
		read -r -p "Control-plane email: " EMAIL
	fi
	[[ -n "${EMAIL// }" ]] || die "email required (or set MASQUE_EMAIL)"
	if [[ -z "${PASSWORD}" ]] && [[ -t 0 ]]; then
		read -r -s -p "Password: " PASSWORD
		echo
	fi
	[[ -n "${PASSWORD}" ]] || die "password required (or set MASQUE_PASSWORD)"

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
		-H 'Accept: application/json' \
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

fix_loopback_masque_in_client_config() {
	if [[ "${SKIP_MASQUE_CONFIG_FIX:-0}" == "1" ]]; then
		return
	fi
	[[ -f "${CONFIG_FILE}" ]] || return
	local target="${MASQUE_SERVER_URL:-}"
	if [[ -z "${target}" ]]; then
		if [[ "${CP}" == *'127.0.0.1'* ]] || [[ "${CP}" == *'localhost'* ]]; then
			return
		fi
		target="${DEFAULT_PUBLIC_MASQUE}"
	fi
	target="${target%/}"
	python3 -c '
import json, sys
path, target = sys.argv[1], sys.argv[2]
with open(path) as f:
    c = json.load(f)
u = (c.get("masque_server_url") or "").lower()
if "127.0.0.1" in u or "localhost" in u or u.startswith("http://0.0.0.0"):
    c["masque_server_url"] = target
    with open(path, "w") as f:
        json.dump(c, f, indent=2)
    print("[quick] fixed masque_server_url in %s -> %s" % (path, target))
' "${CONFIG_FILE}" "${target}"
}

fix_loopback_masque_in_client_config

if [[ "${LEGACY_CONNECT}" == "1" ]]; then
	case "${CONNECT_MODE}" in
		dry-run)
			echo "[quick] LEGACY_CONNECT=1: HTTP connect dry-run..."
			"${MASQUE_CLIENT}" connect -dry-run
			;;
		real)
			echo "[quick] LEGACY_CONNECT=1: HTTP connect (no TUN)..."
			sudo -E env "PATH=${PATH}" "${MASQUE_CLIENT}" connect -check
			;;
		*)
			die "CONNECT_MODE must be dry-run or real (got ${CONNECT_MODE})"
			;;
	esac
	echo "[quick] done."
	exit 0
fi

DNS_CSV="${MASQUE_TUN_DNS:-}"
if [[ -z "${DNS_CSV// }" ]] && [[ -f "${CONFIG_FILE}" ]]; then
	DNS_CSV="$(python3 -c 'import json,sys; c=json.load(open(sys.argv[1])); d=c.get("dns") or []; print(",".join(str(x) for x in d))' "${CONFIG_FILE}" 2>/dev/null || true)"
fi
[[ -n "${DNS_CSV// }" ]] || DNS_CSV="1.1.1.1,8.8.8.8"

# When masque has no QUIC in GET /capabilities, client still needs -connect-ip-udp; infer host from saved HTTP base URL.
infer_connect_ip_udp_from_config() {
	if [[ -n "${CONNECT_IP_UDP:-}" ]]; then
		return
	fi
	if [[ "${AUTO_CONNECT_IP_UDP:-1}" == "0" ]]; then
		return
	fi
	[[ -f "${CONFIG_FILE}" ]] || return
	local ms
	ms="$(python3 -c 'import json,sys; print((json.load(open(sys.argv[1])).get("masque_server_url") or "").strip())' "${CONFIG_FILE}" 2>/dev/null)" || return
	[[ -n "${ms}" ]] || return
	local inferred
	inferred="$(python3 -c '
import sys
from urllib.parse import urlparse
u = urlparse(sys.argv[1])
h = u.hostname
if not h:
    raise SystemExit(1)
port = int(sys.argv[2])
print("%s:%d" % (h, port))
' "${ms}" "${AUTO_CONNECT_IP_UDP_PORT:-8444}")" || return
	CONNECT_IP_UDP="${inferred}"
	echo "[quick] inferred CONNECT_IP_UDP=${CONNECT_IP_UDP} (override with CONNECT_IP_UDP=... or set AUTO_CONNECT_IP_UDP=0)"
}

infer_connect_ip_udp_from_config

TUN_ARGS=( -tun-name "${TUN_NAME}" -route split -apply-routes-from-capsule -dns "${DNS_CSV}" )
if [[ -n "${CONNECT_IP_UDP:-}" ]]; then
	TUN_ARGS+=( -connect-ip-udp "${CONNECT_IP_UDP}" )
else
	echo "[quick] warn: CONNECT_IP_UDP unset and could not infer from ${CONFIG_FILE}; connect-ip-tun needs QUIC. Set CONNECT_IP_UDP or enable QUIC_LISTEN_ADDR on masque-server." >&2
fi

echo "[quick] VPN: interface=${TUN_NAME} dns=${DNS_CSV} udp=${CONNECT_IP_UDP:-<from capabilities — may fail>}"
echo "[quick] starting connect-ip-tun (sudo; Ctrl+C to exit and restore DNS/routes)..."
exec sudo -E env "PATH=${PATH}" "${MASQUE_CLIENT}" connect-ip-tun "${TUN_ARGS[@]}"
