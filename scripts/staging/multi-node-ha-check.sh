#!/usr/bin/env bash
set -euo pipefail

MASQUE_NODE_URLS="${MASQUE_NODE_URLS:-}"
MASQUE_LB_URL="${MASQUE_LB_URL:-}"
PROMETHEUS_URL="${PROMETHEUS_URL:-http://127.0.0.1:9090}"
EXPECTED_HEALTHY_NODES="${EXPECTED_HEALTHY_NODES:-2}"

if [[ -z "${MASQUE_NODE_URLS}" ]]; then
  echo "[multi-node-ha] MASQUE_NODE_URLS is empty. Example: http://10.0.0.11:8443,http://10.0.0.12:8443" >&2
  exit 1
fi

if ! [[ "${EXPECTED_HEALTHY_NODES}" =~ ^[0-9]+$ ]]; then
  echo "[multi-node-ha] EXPECTED_HEALTHY_NODES must be an integer" >&2
  exit 1
fi

echo "[multi-node-ha] prometheus=${PROMETHEUS_URL}"
echo "[multi-node-ha] expected_healthy_nodes=${EXPECTED_HEALTHY_NODES}"
echo "[multi-node-ha] node_urls=${MASQUE_NODE_URLS}"
echo "[multi-node-ha] lb_url=${MASQUE_LB_URL:-<unset>}"

IFS=',' read -r -a nodes <<< "${MASQUE_NODE_URLS}"
if [[ "${#nodes[@]}" -lt "${EXPECTED_HEALTHY_NODES}" ]]; then
  echo "[multi-node-ha] listed nodes (${#nodes[@]}) < EXPECTED_HEALTHY_NODES (${EXPECTED_HEALTHY_NODES})" >&2
  exit 1
fi

healthy_count=0
baseline_caps=""
for raw in "${nodes[@]}"; do
  node="$(echo "${raw}" | xargs)"
  if [[ -z "${node}" ]]; then
    continue
  fi
  echo "[multi-node-ha] check node=${node}"
  curl -fsS "${node%/}/healthz" >/dev/null
  caps="$(curl -fsS "${node%/}/v1/masque/capabilities")"
  if [[ -z "${baseline_caps}" ]]; then
    baseline_caps="${caps}"
  fi
  healthy_count=$((healthy_count + 1))
done

if [[ "${healthy_count}" -lt "${EXPECTED_HEALTHY_NODES}" ]]; then
  echo "[multi-node-ha] healthy node checks (${healthy_count}) < expected (${EXPECTED_HEALTHY_NODES})" >&2
  exit 1
fi

targets_raw="$(curl -fsS "${PROMETHEUS_URL}/api/v1/targets")"
python3 - <<'PY' "${targets_raw}" "${EXPECTED_HEALTHY_NODES}"
import json, sys
d = json.loads(sys.argv[1])
expected = int(sys.argv[2])
targets = d.get("data", {}).get("activeTargets", [])
healthy = [t for t in targets if t.get("labels", {}).get("job") == "masque-server" and t.get("health") == "up"]
if len(healthy) < expected:
    raise SystemExit(f"prometheus healthy masque-server targets={len(healthy)} < expected={expected}")
print(f"[multi-node-ha] prometheus healthy masque-server targets={len(healthy)}")
PY

if [[ -n "${MASQUE_LB_URL}" ]]; then
  lb="${MASQUE_LB_URL%/}"
  echo "[multi-node-ha] check load-balancer endpoint=${lb}"
  curl -fsS "${lb}/healthz" >/dev/null
  lb_caps="$(curl -fsS "${lb}/v1/masque/capabilities")"
  python3 - <<'PY' "${baseline_caps}" "${lb_caps}"
import json, sys
node_cap = json.loads(sys.argv[1])
lb_cap = json.loads(sys.argv[2])

def get_keys(cap):
    ci = (((cap.get("tunnel") or {}).get("quic") or {}).get("connect_ip") or {})
    dg = (ci.get("http3_datagrams") or {})
    return {
        "tun_linux_per_session": bool(dg.get("tun_linux_per_session", False)),
        "tun_linux_managed_nat": bool(dg.get("tun_linux_managed_nat", False)),
        "tun_linux_shared": bool(dg.get("tun_linux_shared", False)),
        "tun_linux_managed_nat_backend": str(dg.get("tun_linux_managed_nat_backend", "")),
    }

node_keys = get_keys(node_cap)
lb_keys = get_keys(lb_cap)
if node_keys != lb_keys:
    raise SystemExit(f"lb capability mismatch vs node baseline: node={node_keys} lb={lb_keys}")
print("[multi-node-ha] lb capability profile matches node baseline")
PY
fi

echo "[multi-node-ha] check passed"

