#!/usr/bin/env bash
# Docker-only: register device (bootstrap), bring up connect-ip-tun, curl internet, optional speedtest.
# Requires: root, /dev/net/tun, masque with QUIC + CONNECT_IP_TUN_* (see docker/e2e/docker-compose.yml).
set -euo pipefail

CP="${CONTROL_PLANE_URL:-http://control-plane:8000}"
CP="${CP%/}"
MS="${MASQUE_SERVER_URL:-http://masque:8443}"
MS="${MS%/}"

TUNPID=""
WORKDIR="$(mktemp -d)"

cleanup() {
	if [[ -n "${TUNPID}" ]] && kill -0 "${TUNPID}" 2>/dev/null; then
		kill -INT "${TUNPID}" 2>/dev/null || true
		wait "${TUNPID}" 2>/dev/null || true
	fi
	rm -rf "${WORKDIR}" 2>/dev/null || true
}
trap cleanup EXIT

if [[ "$(id -u)" != "0" ]]; then
	echo "error: must run as root (TUN + routes + resolv.conf)" >&2
	exit 1
fi

echo "[vpn-probe] control-plane=${CP} masque=${MS}"

curl -fsS "${CP}/api/v1/health" >/dev/null
curl -fsS "${MS}/healthz" >/dev/null

export MASQUE_CLIENT_CONFIG="${WORKDIR}/client.json"
export MASQUE_CLIENT_STATE="${WORKDIR}/state.json"
export MASQUE_FINGERPRINT_FILE="${WORKDIR}/device-fingerprint"

EMAIL="vpn-$(date +%s)@e2e.test"
PASS="${VPN_PROBE_PASSWORD:-password123}"

curl -fsS -X POST "${CP}/api/v1/users" \
	-H 'Content-Type: application/json' \
	-d "{\"name\":\"VPN Probe\",\"email\":\"${EMAIL}\",\"password\":\"${PASS}\"}" >/dev/null
echo "[vpn-probe] created user ${EMAIL}"

export MASQUE_PASSWORD="${PASS}"
masque-client quick-login -control-plane "${CP}" -email "${EMAIL}"
echo "[vpn-probe] bootstrap OK (config + fingerprint)"

masque-client connect -dry-run

echo "[vpn-probe] starting connect-ip-tun (background)…"
masque-client connect-ip-tun \
	-masque-server "${MS}" \
	-route split \
	-apply-routes-from-capsule \
	-dns "1.1.1.1,8.8.8.8" \
	-reconnect-max-dial-failures 15 \
	-reconnect-max-session-drops 5 \
	2> "${WORKDIR}/tun.log" &
TUNPID=$!

ready=0
for _ in $(seq 1 45); do
	if ip link show tun0 >/dev/null 2>&1; then
		ready=1
		break
	fi
	if ! kill -0 "${TUNPID}" 2>/dev/null; then
		echo "[vpn-probe] connect-ip-tun exited before tun0 appeared:" >&2
		cat "${WORKDIR}/tun.log" >&2 || true
		exit 1
	fi
	sleep 1
done
if [[ "${ready}" != "1" ]]; then
	echo "[vpn-probe] timeout waiting for tun0" >&2
	cat "${WORKDIR}/tun.log" >&2 || true
	exit 1
fi
echo "[vpn-probe] tun0 is up"

echo ""
echo "=== curl https://ipinfo.io/ip (egress IPv4) ==="
curl -4sS --max-time 40 "https://ipinfo.io/ip" | tee "${WORKDIR}/ip.txt"
echo ""

echo ""
echo "=== curl https://ipinfo.io/json (truncated) ==="
curl -4sS --max-time 40 "https://ipinfo.io/json" | head -c 800 || true
echo ""

echo ""
echo "=== curl -vvv -L https://www.google.com/ (TCP/TLS, redirects; tail of verbose log) ==="
set +e
curl -4 -sS --max-time 90 -vvv -L "https://www.google.com/" -o /dev/null 2>&1 | tail -n 50
curl_rc=$?
set -e
if [[ "${curl_rc}" != "0" ]]; then
	echo "[vpn-probe] WARN: google HTTPS curl exit ${curl_rc} (may be regional/filtering)" >&2
fi

echo ""
echo "=== curl -vvv -L http://google.com/ (TCP port 80 → redirect chain) ==="
set +e
curl -4 -sS --max-time 90 -vvv -L "http://google.com/" -o /dev/null 2>&1 | tail -n 35
curl_rc2=$?
set -e
if [[ "${curl_rc2}" != "0" ]]; then
	echo "[vpn-probe] WARN: google HTTP curl exit ${curl_rc2}" >&2
fi

if [[ "${SKIP_SPEEDTEST:-0}" == "1" ]]; then
	echo "[vpn-probe] SKIP_SPEEDTEST=1 — skipping speedtest-cli"
else
	echo ""
	echo "=== speedtest-cli --simple (timeout 180s; set SKIP_SPEEDTEST=1 to skip) ==="
	if command -v speedtest-cli >/dev/null 2>&1; then
		set +e
		timeout 180 speedtest-cli --simple
		st_rc=$?
		set -e
		if [[ "${st_rc}" != "0" ]]; then
			echo "[vpn-probe] WARN: speedtest-cli exit ${st_rc} (non-fatal)" >&2
		fi
	else
		echo "[vpn-probe] WARN: speedtest-cli not in PATH" >&2
	fi
fi

echo ""
echo "[vpn-probe] all checks finished OK"
