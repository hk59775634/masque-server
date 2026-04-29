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
# Env:
#  MASQUE_HOST, CLIENT_HOST, RUN_BENCH (1=run bench when not skipped)
#  FAULT_RESTORE_ONLY=1   Same as --restore-only (after CLI flags).
#
set -euo pipefail

MASQUE_HOST="${MASQUE_HOST:-103.6.4.131}"
CLIENT_HOST="${CLIENT_HOST:-103.6.4.5}"
RUN_BENCH="${RUN_BENCH:-1}"

REMOTE_DROPIN="/etc/systemd/system/masque-server.service.d/vpn-public.conf"
REMOTE_BACKUP="/tmp/vpn-public.conf.faultinj.bak"

DRY_RUN=0
RESTORE_ONLY=0
SKIP_INJECT=0
SKIP_BENCH=0
SKIP_METRICS=0
SKIP_LOGS=0
LEAVE_CHANGED=0

usage() {
  cat <<'EOF'
Fault-injection helper: force nft managed NAT with iptables fallback off, then restore.

Usage: vpn-nat-backend-fault-injection.sh [options]

Options:
  --dry-run          Print planned steps and exit (no SSH).
  --restore-only     Only restore drop-in from remote backup if present.
  --skip-inject      Do not modify remote config or restart (metrics/logs/bench only).
  --skip-bench       Do not run vpn-tunnel-bench.sh.
  --skip-metrics     Do not scrape /metrics for managed-NAT counters.
  --skip-logs        Do not print filtered journal lines.
  --leave-changed    After inject, do not restore; clears EXIT trap (dangerous).

Examples:
  ./vpn-nat-backend-fault-injection.sh --dry-run
  ./vpn-nat-backend-fault-injection.sh --restore-only
  FAULT_RESTORE_ONLY=1 ./vpn-nat-backend-fault-injection.sh
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    -h | --help)
      usage
      exit 0
      ;;
    --dry-run) DRY_RUN=1 ;;
    --restore-only) RESTORE_ONLY=1 ;;
    --skip-inject) SKIP_INJECT=1 ;;
    --skip-bench) SKIP_BENCH=1 ;;
    --skip-metrics) SKIP_METRICS=1 ;;
    --skip-logs) SKIP_LOGS=1 ;;
    --leave-changed) LEAVE_CHANGED=1 ;;
    *)
      echo "unknown option: $1" >&2
      usage >&2
      exit 1
      ;;
  esac
  shift
done

if [[ "${FAULT_RESTORE_ONLY:-0}" == "1" ]]; then
  RESTORE_ONLY=1
fi

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

run_inject() {
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
}

run_metrics() {
  ssh "root@${MASQUE_HOST}" "
    curl -sS --max-time 5 http://127.0.0.1:8443/metrics | awk '
      /masque_connect_ip_tun_managed_nat_apply_total/ {print}
      /masque_connect_ip_tun_managed_nat_backend_total/ {print}
      /masque_connect_ip_tun_open_echo_fallback_total/ {print}
    '
  "
}

run_logs() {
  ssh "root@${MASQUE_HOST}" "
    journalctl -u masque-server -n 80 --no-pager | awk '/managed NAT|nft|fallback|shared TUN unavailable|open_echo_fallback/ {print}'
  "
}

echo "[fault] host=${MASQUE_HOST} client=${CLIENT_HOST}"

if [[ "${DRY_RUN}" == "1" ]]; then
  echo "[fault] dry-run: would SSH to root@${MASQUE_HOST}"
  if [[ "${RESTORE_ONLY}" == "1" ]]; then
    echo "[fault] dry-run: restore-only → restore_remote if ${REMOTE_BACKUP} exists"
    exit 0
  fi
  if [[ "${SKIP_INJECT}" == "0" ]]; then
    echo "[fault] dry-run: backup drop-in, inject nft-only + no iptables fallback, restart masque-server"
  else
    echo "[fault] dry-run: skip inject"
  fi
  if [[ "${SKIP_BENCH}" == "0" && "${RUN_BENCH}" == "1" ]]; then
    echo "[fault] dry-run: run vpn-tunnel-bench.sh (CLIENT_HOST=${CLIENT_HOST})"
  else
    echo "[fault] dry-run: skip bench"
  fi
  if [[ "${SKIP_METRICS}" == "0" ]]; then
    echo "[fault] dry-run: print managed-NAT metrics from :8443/metrics"
  fi
  if [[ "${SKIP_LOGS}" == "0" ]]; then
    echo "[fault] dry-run: print filtered journalctl lines"
  fi
  if [[ "${LEAVE_CHANGED}" == "1" ]]; then
    echo "[fault] dry-run: leave remote changed (no restore)"
  else
    echo "[fault] dry-run: restore drop-in and restart (unless inject skipped and nothing to restore)"
  fi
  exit 0
fi

if [[ "${RESTORE_ONLY}" == "1" ]]; then
  echo "[fault] restore-only"
  restore_remote
  echo "[fault] done"
  exit 0
fi

trap restore_remote EXIT

if [[ "${SKIP_INJECT}" == "0" ]]; then
  echo "[fault] injecting nft-only managed NAT (fallback iptables disabled)"
  run_inject
else
  echo "[fault] skip inject"
fi

if [[ "${SKIP_BENCH}" == "0" && "${RUN_BENCH}" == "1" ]]; then
  echo "[fault] running vpn-tunnel-bench in nft-only mode"
  CLIENT_HOST="${CLIENT_HOST}" SKIP_ACTIVATE=1 INSTALL_SPEEDTEST=0 SKIP_PUBLIC_IPERF=1 CURL_GOOGLE_MAX=35 \
    "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/vpn-tunnel-bench.sh"
else
  echo "[fault] skip bench"
fi

if [[ "${SKIP_METRICS}" == "0" ]]; then
  echo "[fault] managed NAT backend metrics"
  run_metrics
fi

if [[ "${SKIP_LOGS}" == "0" ]]; then
  echo "[fault] recent managed-NAT logs"
  run_logs
fi

if [[ "${LEAVE_CHANGED}" == "1" ]]; then
  echo "[fault] leave-changed: remote left in injected state; run with --restore-only to revert"
  trap - EXIT
  exit 0
fi

echo "[fault] restoring original config"
restore_remote
trap - EXIT
echo "[fault] done"
