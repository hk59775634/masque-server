#!/usr/bin/env bash
set -euo pipefail

MASQUE_SERVER_URL="${MASQUE_SERVER_URL:-http://127.0.0.1:8443}"
PROMETHEUS_URL="${PROMETHEUS_URL:-http://127.0.0.1:9090}"
ALERTMANAGER_URL="${ALERTMANAGER_URL:-http://127.0.0.1:9093}"

echo "[phase2b-kernel] masque_server=${MASQUE_SERVER_URL}"
echo "[phase2b-kernel] prometheus=${PROMETHEUS_URL}"
echo "[phase2b-kernel] alertmanager=${ALERTMANAGER_URL}"

caps_raw="$(curl -fsS "${MASQUE_SERVER_URL}/v1/masque/capabilities")"
python3 - <<'PY' "${caps_raw}"
import json, sys

cap = json.loads(sys.argv[1])
ci = (((cap.get("tunnel") or {}).get("quic") or {}).get("connect_ip") or {}
dg = (ci.get("http3_datagrams") or {})
required = ["tun_linux_per_session", "tun_linux_managed_nat", "tun_linux_shared"]
missing = [k for k in required if not dg.get(k, False)]
if missing:
    raise SystemExit(f"missing expected capabilities flags: {missing}")
dev = ci.get("dev") or {}
for key in ("tun_forward_env", "tun_managed_nat_env", "tun_shared_env", "tun_shared_ttl_env"):
    if key not in dev:
        raise SystemExit(f"missing connect_ip.dev.{key}")
print("[phase2b-kernel] capabilities flags OK")
PY

metrics_raw="$(curl -fsS "${MASQUE_SERVER_URL}/metrics")"
python3 - <<'PY' "${metrics_raw}"
import sys
body = sys.argv[1]
must_have = [
    "masque_connect_ip_tun_bridge_active",
    "masque_connect_ip_tun_open_echo_fallback_total",
    "masque_connect_ip_tun_link_up_failures_total",
    "masque_connect_ip_tun_managed_nat_apply_total",
    "masque_connect_ip_tun_shared_binding_conflicts_total",
    "masque_connect_ip_tun_shared_binding_stale_evictions_total",
]
missing = [m for m in must_have if m not in body]
if missing:
    raise SystemExit(f"missing expected metrics: {missing}")
print("[phase2b-kernel] metrics names OK")
PY

rules_raw="$(curl -fsS "${PROMETHEUS_URL}/api/v1/rules")"
python3 - <<'PY' "${rules_raw}"
import json, sys
r = json.loads(sys.argv[1])
names = {rule.get("name","") for g in r.get("data", {}).get("groups", []) for rule in g.get("rules", [])}
required = {
    "MasqueConnectIPTunOpenEchoFallback",
    "MasqueConnectIPTunLinkUpFailures",
    "MasqueConnectIPTunManagedNATApplyErrors",
    "MasqueConnectIPTunSharedBindingConflictsHigh",
}
missing = sorted(required - names)
if missing:
    raise SystemExit(f"missing expected alert rules: {missing}")
print("[phase2b-kernel] prometheus alert rules OK")
PY

if curl -fsS "${ALERTMANAGER_URL}/api/v2/status" >/dev/null; then
  echo "[phase2b-kernel] alertmanager API reachable"
else
  echo "[phase2b-kernel] FAIL alertmanager API unreachable"
  exit 1
fi

echo "[phase2b-kernel] checks passed."

