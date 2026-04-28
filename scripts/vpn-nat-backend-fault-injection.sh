#!/usr/bin/env bash
#
# Fault-injection helper for managed NAT backend behavior on masque-server.
# Default target host: 103.6.4.131
#
# Flow:
#  1) Backup vpn-public.conf drop-in.
#  2) Force nft backend with fallback disabled.
#  3) Restart masque-server and run vpn-tunnel-bench.sh once.
#  4) Print key managed-NAT metrics and recent logs.
#  5) Restore original drop-in and restart service.
#
set -euo pipefail

MASQUE_HOST="${MASQUE_HOST:-103.6.4.131}"
CLIENT_HOST="${CLIENT_HOST:-103.6.4.5}"
RUN_BENCH="${RUN_BENCH:-1}"

REMOTE_DROPIN="/etc/systemd/system/masque-server.service.d/vpn-public.conf"
REMOTE_BACKUP="/tmp/vpn-public.conf.faultinj.bak"

echo "[fault] host=${MASQUE_HOST} client=${CLIENT_HOST}"

restore_remote() {
  ssh "root@${MASQUE_HOST}" "
    set -e
    if [[ -f '${REMOTE_BACKUP}' ]]; then
      cp -a '${REMOTE_BACKUP}' '${REMOTE_DROPIN}'
      systemctl daemon-reload
      systemctl restart masque-server
      rm -f '${REMOTE_BACKUP}'
    fi
  " >/dev/null 2>&1 || true
}
trap restore_remote EXIT

ssh "root@${MASQUE_HOST}" "
  set -euo pipefail
  cp -a '${REMOTE_DROPIN}' '${REMOTE_BACKUP}'
  awk '
    /CONNECT_IP_TUN_NAT_BACKEND=/ {next}
    /CONNECT_IP_TUN_NAT_FALLBACK_IPTABLES=/ {next}
    {print}
    /CONNECT_IP_TUN_MANAGED_NAT=1/ {
      print \"Environment=CONNECT_IP_TUN_NAT_BACKEND=nftables\"
      print \"Environment=CONNECT_IP_TUN_NAT_FALLBACK_IPTABLES=0\"
    }
  ' '${REMOTE_DROPIN}' > '${REMOTE_DROPIN}.new'
  mv '${REMOTE_DROPIN}.new' '${REMOTE_DROPIN}'
  systemctl daemon-reload
  systemctl restart masque-server
  systemctl is-active masque-server
"

if [[ "${RUN_BENCH}" == "1" ]]; then
  echo "[fault] running vpn-tunnel-bench in nft-only mode"
  CLIENT_HOST="${CLIENT_HOST}" SKIP_ACTIVATE=1 INSTALL_SPEEDTEST=0 SKIP_PUBLIC_IPERF=1 CURL_GOOGLE_MAX=35 \
    "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/vpn-tunnel-bench.sh"
fi

echo "[fault] managed NAT backend metrics"
ssh "root@${MASQUE_HOST}" "
  curl -sS --max-time 5 http://127.0.0.1:8443/metrics | awk '
    /masque_connect_ip_tun_managed_nat_apply_total/ {print}
    /masque_connect_ip_tun_managed_nat_backend_total/ {print}
    /masque_connect_ip_tun_open_echo_fallback_total/ {print}
  '
"

echo "[fault] recent managed-NAT logs"
ssh "root@${MASQUE_HOST}" "
  journalctl -u masque-server -n 80 --no-pager | awk '/managed NAT|nft|fallback|shared TUN unavailable|open_echo_fallback/ {print}'
"

echo "[fault] restoring original config"
restore_remote
trap - EXIT
echo "[fault] done"

