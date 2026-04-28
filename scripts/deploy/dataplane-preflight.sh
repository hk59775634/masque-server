#!/usr/bin/env bash
set -euo pipefail

TAG="${1:-preflight}"

ok() { echo "[${TAG}] [OK] $*"; }
warn() { echo "[${TAG}] [WARN] $*" >&2; }
die() { echo "[${TAG}] [ERROR] $*" >&2; exit 1; }

for bin in ip sysctl; do
  command -v "${bin}" >/dev/null 2>&1 || die "required binary not found: ${bin}"
done
ok "core networking tools present (ip, sysctl)"

if command -v nft >/dev/null 2>&1; then
  ok "nftables binary found"
else
  warn "nft not found; managed NAT nftables backend unavailable (fallback to iptables if enabled)"
fi

if command -v iptables >/dev/null 2>&1; then
  ok "iptables binary found"
else
  warn "iptables not found; fallback backend unavailable"
fi

if [[ -e /dev/net/tun ]]; then
  ok "/dev/net/tun present"
else
  warn "/dev/net/tun missing; CONNECT_IP_TUN_FORWARD will fail and may fallback to echo mode"
fi

if [[ "${EUID:-0}" -eq 0 ]]; then
  ok "running as root"
else
  warn "not root; deploy may rely on sudo/systemctl permissions"
fi

if command -v systemctl >/dev/null 2>&1 && systemctl status masque-server >/dev/null 2>&1; then
  env_line="$(systemctl show masque-server -p Environment --no-pager 2>/dev/null || true)"
  if [[ -n "${env_line}" ]]; then
    echo "[${TAG}] masque-server env: ${env_line#Environment=}"
  fi
fi

