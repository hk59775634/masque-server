#!/usr/bin/env bash
# Full client bench on 103.6.4.5: issue activation (local CP) → activate → doctor → connect-ip-tun → curl + iperf → teardown.
set -euo pipefail

CLIENT_HOST="${CLIENT_HOST:-103.6.4.5}"
CP_PUBLIC="${CP_PUBLIC:-https://www.afbuyers.com}"
CP_LOCAL="${CP_LOCAL:-http://127.0.0.1:8081}"
MASQUE_HTTP="${MASQUE_HTTP:-http://103.6.4.131:8443}"
MASQUE_UDP="${MASQUE_UDP:-103.6.4.131:8444}"
USER_ID="${USER_ID:-1}"

FP="fp-bench-$(date +%s)-$(openssl rand -hex 4)"
echo "[local] issue activation fingerprint=$FP"
AC_JSON="$(curl -sS -X POST "${CP_LOCAL}/api/v1/devices/activation-code" \
  -H 'Content-Type: application/json' \
  -d "$(jq -nc --arg fp "$FP" --arg dn "bench-$(hostname -s)" --argjson uid "$USER_ID" \
    '{user_id: $uid, device_name: $dn, fingerprint: $fp}')")"
CODE="$(echo "$AC_JSON" | jq -r .activation_code)"
if [[ -z "$CODE" || "$CODE" == "null" ]]; then
  echo "activation issue failed: $AC_JSON" >&2
  exit 1
fi
echo "[local] activation_code=$CODE"

echo "[local] iperf3 server on masque (background)"
pkill -x iperf3 2>/dev/null || true
sleep 0.3
iperf3 -s >/tmp/iperf3-masque.log 2>&1 &
sleep 0.4

echo "[remote] activate, doctor, tunnel, curl, iperf"
ssh -o BatchMode=yes -o StrictHostKeyChecking=accept-new "root@${CLIENT_HOST}" \
  FP="$FP" CODE="$CODE" CP_PUBLIC="$CP_PUBLIC" MASQUE_HTTP="$MASQUE_HTTP" MASQUE_UDP="$MASQUE_UDP" \
  bash -s <<'REMOTE'
set -euo pipefail
export PATH="/usr/local/bin:${PATH:-}"

pkill -f 'masque-client connect-ip-tun' 2>/dev/null || true
sleep 1

echo "=== activate ==="
masque-client activate -control-plane "${CP_PUBLIC}" -fingerprint "${FP}" -code "${CODE}"

jq --arg m "${MASQUE_HTTP}" '.masque_server_url = $m' /root/.masque-client.json > /tmp/mc.json && mv /tmp/mc.json /root/.masque-client.json
chmod 600 /root/.masque-client.json

echo "=== config show (partial) ==="
masque-client config show | head -15

echo "=== doctor ==="
masque-client doctor -control-plane "${CP_PUBLIC}" -masque-server "${MASQUE_HTTP}" -connect-ip -connect-ip-udp "${MASQUE_UDP}"

rm -f /tmp/tun.log /tmp/tun.pid
echo "=== connect-ip-tun (bg) ==="
timeout 120 masque-client connect-ip-tun \
  -masque-server "${MASQUE_HTTP}" \
  -connect-ip-udp "${MASQUE_UDP}" \
  -route split \
  -dns '1.1.1.1,8.8.8.8' \
  -quic-max-idle 30m -quic-keepalive 15s \
  > /tmp/tun.log 2>&1 &
echo $! > /tmp/tun.pid

for i in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15; do
  if ip link show tun0 >/dev/null 2>&1; then break; fi
  sleep 1
done
if ! ip link show tun0 >/dev/null 2>&1; then
  echo "tun0 failed to appear" >&2
  tail -100 /tmp/tun.log >&2 || true
  kill "$(cat /tmp/tun.pid)" 2>/dev/null || true
  exit 1
fi

ip -br link show tun0
ip addr show dev tun0 | head -8

echo "--- curl via tunnel (cloudflare 1MB) ---"
curl -4 -sS -o /dev/null -w "curl_cf_1mb: time_total=%{time_total}s size=%{size_download} http_code=%{http_code}\n" --max-time 40 "https://speed.cloudflare.com/__down?bytes=1000000"

echo "--- curl -vvv -L google.com (IPv4 only; 避免 VPN 下 IPv6/AAAA 卡住) ---"
rm -f /tmp/curl-google-body.html /tmp/curl-google-verbose.log
set +e
timeout 60 curl -4 -vvv -L --max-time 55 \
  -o /tmp/curl-google-body.html \
  -w "\n--- curl_writeout ---\nhttp_code=%{http_code} num_redirects=%{num_redirects} time_total=%{time_total} size_download=%{size_download}\n" \
  http://google.com > /tmp/curl-google-verbose.log 2>&1
curl_g_rc=$?
set -e
tail -n 120 /tmp/curl-google-verbose.log || true
[[ "$curl_g_rc" == "0" ]] || echo "[warn] curl google exit=$curl_g_rc"

echo "--- iperf3 via tunnel (103.6.4.131:5201, 2 streams, 12s) ---"
iperf3 -c 103.6.4.131 -t 12 -P 2

echo "--- iperf3 reverse (8s) ---"
iperf3 -c 103.6.4.131 -t 8 -R

kill "$(cat /tmp/tun.pid)" 2>/dev/null || true
sleep 2
pkill -f 'masque-client connect-ip-tun' 2>/dev/null || true

echo "=== tun log tail ==="
tail -35 /tmp/tun.log || true
REMOTE

pkill -x iperf3 2>/dev/null || true
echo "[local] done"
