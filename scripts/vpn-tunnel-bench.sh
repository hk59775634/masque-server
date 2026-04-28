#!/usr/bin/env bash
#
# VPN 通道压测（在 SSH 目标机上）：masque-client connect-ip-tun 起隧道后，使用公网 curl / iperf3 / speedtest-cli。
#
# 在 masque 主机上执行（需能 SSH 到客户端，且本机可 POST 控制面发码，除非 SKIP_ACTIVATE=1）：
#   CLIENT_HOST=103.6.4.5 ./scripts/vpn-tunnel-bench.sh
#
# 环境变量：
#   CLIENT_HOST          SSH 目标（默认 103.6.4.5）
#   CP_PUBLIC            控制面 HTTPS（默认 https://www.afbuyers.com）
#   CP_LOCAL             本机控制面发 activation-code（默认 http://127.0.0.1:8081）
#   MASQUE_HTTP / MASQUE_UDP  直连 masque（默认 http://103.6.4.131:8443 / 103.6.4.131:8444）
#   USER_ID              发码用 user id（默认 1）
#   SKIP_ACTIVATE=1      跳过发码与 activate（远端需已有 /root/.masque-client.json）
#   INSTALL_SPEEDTEST=1  远端若无 speedtest-cli 则 apt-get install（默认 1）
#   INCLUDE_LOCAL_IPERF=1  额外跑 iperf3 到 MASQUE_IP:5201（需本机先 iperf3 -s；默认 0）
#   MASQUE_IP            内网 iperf 目标（默认 103.6.4.131）
#   SKIP_PUBLIC_IPERF=1  跳过公网 iperf3（部分网络/旧 iperf3 易卡住；默认 0）
#   CURL_GOOGLE_MAX     curl google 墙钟超时秒数（默认 60）
#
set -euo pipefail

CLIENT_HOST="${CLIENT_HOST:-103.6.4.5}"
CP_PUBLIC="${CP_PUBLIC:-https://www.afbuyers.com}"
CP_LOCAL="${CP_LOCAL:-http://127.0.0.1:8081}"
MASQUE_HTTP="${MASQUE_HTTP:-http://103.6.4.131:8443}"
MASQUE_UDP="${MASQUE_UDP:-103.6.4.131:8444}"
MASQUE_IP="${MASQUE_IP:-103.6.4.131}"
USER_ID="${USER_ID:-1}"
SKIP_ACTIVATE="${SKIP_ACTIVATE:-0}"
INSTALL_SPEEDTEST="${INSTALL_SPEEDTEST:-1}"
INCLUDE_LOCAL_IPERF="${INCLUDE_LOCAL_IPERF:-0}"
SKIP_PUBLIC_IPERF="${SKIP_PUBLIC_IPERF:-0}"
CURL_GOOGLE_MAX="${CURL_GOOGLE_MAX:-60}"
TUN_WAIT_SEC="${TUN_WAIT_SEC:-20}"

FP=""
CODE=""
if [[ "$SKIP_ACTIVATE" != "1" ]]; then
  FP="fp-vpn-$(date +%s)-$(openssl rand -hex 4)"
  echo "[local] issue activation fingerprint=$FP"
  AC_JSON="$(curl -sS -X POST "${CP_LOCAL}/api/v1/devices/activation-code" \
    -H 'Content-Type: application/json' \
    -d "$(jq -nc --arg fp "$FP" --arg dn "vpn-bench-$(hostname -s)" --argjson uid "$USER_ID" \
      '{user_id: $uid, device_name: $dn, fingerprint: $fp}')")"
  CODE="$(echo "$AC_JSON" | jq -r .activation_code)"
  if [[ -z "$CODE" || "$CODE" == "null" ]]; then
    echo "activation issue failed: $AC_JSON" >&2
    exit 1
  fi
  echo "[local] activation_code=$CODE"
fi

if [[ "$INCLUDE_LOCAL_IPERF" == "1" ]]; then
  echo "[local] iperf3 -s on masque (background)"
  pkill -x iperf3 2>/dev/null || true
  sleep 0.2
  iperf3 -s >/tmp/iperf3-masque.log 2>&1 &
  sleep 0.4
fi

ssh -o BatchMode=yes -o StrictHostKeyChecking=accept-new "root@${CLIENT_HOST}" \
  FP="${FP:-}" CODE="${CODE:-}" CP_PUBLIC="${CP_PUBLIC}" MASQUE_HTTP="${MASQUE_HTTP}" MASQUE_UDP="${MASQUE_UDP}" \
  SKIP_ACTIVATE="${SKIP_ACTIVATE}" INSTALL_SPEEDTEST="${INSTALL_SPEEDTEST}" \
  INCLUDE_LOCAL_IPERF="${INCLUDE_LOCAL_IPERF}" SKIP_PUBLIC_IPERF="${SKIP_PUBLIC_IPERF}" \
  MASQUE_IP="${MASQUE_IP}" TUN_WAIT_SEC="${TUN_WAIT_SEC}" CURL_GOOGLE_MAX="${CURL_GOOGLE_MAX}" \
  bash -s <<'REMOTE'
set -euo pipefail
export PATH="/usr/local/bin:${PATH:-}"

pkill -f 'masque-client connect-ip-tun' 2>/dev/null || true
sleep 1

if [[ "${SKIP_ACTIVATE}" != "1" ]]; then
  echo "=== activate ==="
  masque-client activate -control-plane "${CP_PUBLIC}" -fingerprint "${FP}" -code "${CODE}"
else
  echo "=== skip activate (existing config) ==="
  if [[ ! -f /root/.masque-client.json ]]; then
    echo "missing /root/.masque-client.json" >&2
    exit 1
  fi
fi

jq --arg m "${MASQUE_HTTP}" '.masque_server_url = $m' /root/.masque-client.json > /tmp/mc.json && mv /tmp/mc.json /root/.masque-client.json
chmod 600 /root/.masque-client.json

echo "=== doctor (smoke) ==="
masque-client doctor -control-plane "${CP_PUBLIC}" -masque-server "${MASQUE_HTTP}" -connect-ip -connect-ip-udp "${MASQUE_UDP}" || true

rm -f /tmp/tun-vpn.log /tmp/tun-vpn.pid
echo "=== connect-ip-tun (background) ==="
timeout 600 masque-client connect-ip-tun \
  -masque-server "${MASQUE_HTTP}" \
  -connect-ip-udp "${MASQUE_UDP}" \
  -route split \
  -dns '1.1.1.1,8.8.8.8' \
  -quic-max-idle 30m -quic-keepalive 15s \
  > /tmp/tun-vpn.log 2>&1 &
echo $! > /tmp/tun-vpn.pid

ok=0
for ((i=1; i<=TUN_WAIT_SEC; i++)); do
  if ip link show tun0 >/dev/null 2>&1; then ok=1; break; fi
  sleep 1
done
if [[ "$ok" != "1" ]]; then
  echo "tun0 failed to appear within ${TUN_WAIT_SEC}s" >&2
  tail -120 /tmp/tun-vpn.log >&2 || true
  kill "$(cat /tmp/tun-vpn.pid)" 2>/dev/null || true
  exit 1
fi

ip -br link show tun0
ip addr show dev tun0 | head -6

echo "=== Public: Cloudflare 1MB HTTPS ==="
curl -4 -sS -o /dev/null -w "curl_cf_1mb: time_total=%{time_total}s size=%{size_download} http_code=%{http_code}\n" --max-time 45 "https://speed.cloudflare.com/__down?bytes=1000000"

echo "=== Public: Cloudflare trace (egress IP) ==="
curl -4 -sS --max-time 20 "https://1.1.1.1/cdn-cgi/trace" | head -12

echo "=== Public: Google generate_204 (small) ==="
curl -4 -sS -o /dev/null -w "curl_google: time_total=%{time_total}s http_code=%{http_code}\n" --max-time 20 "http://connectivitycheck.gstatic.com/generate_204" || true

echo "=== Public: curl -vvv -L google.com (强制 -4，避免 AAAA/IPv6 在 VPN 下卡死) ==="
rm -f /tmp/curl-google-body.html /tmp/curl-google-verbose.log
set +e
timeout "${CURL_GOOGLE_MAX}" curl -4 -vvv -L --max-time "${CURL_GOOGLE_MAX}" \
  -o /tmp/curl-google-body.html \
  -w "\n--- curl_writeout ---\nhttp_code=%{http_code} num_redirects=%{num_redirects} time_namelookup=%{time_namelookup} time_connect=%{time_connect} time_appconnect=%{time_appconnect} time_total=%{time_total} size_download=%{size_download}\n" \
  http://google.com > /tmp/curl-google-verbose.log 2>&1
curl_google_rc=$?
set -e
tail -n 150 /tmp/curl-google-verbose.log || true
if [[ "$curl_google_rc" != "0" ]]; then
  echo "[warn] curl google exit=$curl_google_rc (124=timeout)"
fi
if [[ -f /tmp/curl-google-body.html ]]; then
  echo "--- body head (text, 400 bytes) ---"
  head -c 400 /tmp/curl-google-body.html 2>/dev/null | cat -v | head -20 || true
fi

# 公网 iperf3 测点（尽力而为；运营商/对端可能拒绝、限速或握手很慢）
if [[ "${SKIP_PUBLIC_IPERF}" != "1" ]]; then
  echo "=== Public iperf3 (best-effort mirrors, TCP) ==="
  try_public_iperf3() {
    local host="$1" port="${2:-5201}" dur="${3:-8}"
    echo "--- ${host}:${port} ${dur}s ---"
    if timeout "$((dur + 22))" iperf3 -4 -c "$host" -p "$port" -t "$dur" -P 2 2>&1; then
      return 0
    fi
    echo "[warn] failed: ${host}:${port}"
    return 1
  }
  try_public_iperf3 speedtest.tele2.net 5201 8 || true
  try_public_iperf3 ping.online.net 5201 8 || true
  try_public_iperf3 bouygues.iperf.fr 5201 8 || true
else
  echo "=== skip public iperf3 (SKIP_PUBLIC_IPERF=1) ==="
fi

if [[ "${INCLUDE_LOCAL_IPERF}" == "1" ]]; then
  echo "=== Local masque iperf (${MASQUE_IP}:5201) ==="
  iperf3 -c "${MASQUE_IP}" -t 10 -P 2 || true
  iperf3 -c "${MASQUE_IP}" -t 6 -R || true
fi

if [[ "${INSTALL_SPEEDTEST}" == "1" ]] && ! command -v speedtest-cli >/dev/null 2>&1; then
  echo "=== apt install speedtest-cli (non-interactive) ==="
  export DEBIAN_FRONTEND=noninteractive
  apt-get update -qq && apt-get install -y speedtest-cli || echo "[warn] apt install speedtest-cli failed"
fi

if command -v speedtest-cli >/dev/null 2>&1; then
  echo "=== speedtest-cli (python; --mini 更快，仍依赖 speedtest.net API) ==="
  timeout 180 speedtest-cli --simple --mini --timeout 60 || \
  timeout 180 speedtest-cli --simple --timeout 90 || \
  timeout 180 speedtest-cli --simple || true
else
  echo "[skip] speedtest-cli not installed"
fi

kill "$(cat /tmp/tun-vpn.pid)" 2>/dev/null || true
sleep 2
pkill -f 'masque-client connect-ip-tun' 2>/dev/null || true

echo "=== tun log tail ==="
tail -40 /tmp/tun-vpn.log || true
REMOTE

if [[ "$INCLUDE_LOCAL_IPERF" == "1" ]]; then
  pkill -x iperf3 2>/dev/null || true
fi

echo "[local] vpn-tunnel-bench done"
