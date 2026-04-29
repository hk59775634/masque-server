#!/usr/bin/env bash
set -euo pipefail

MASQUE_NODE_URLS="${MASQUE_NODE_URLS:-}"
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

IFS=',' read -r -a nodes <<< "${MASQUE_NODE_URLS}"
if [[ "${#nodes[@]}" -lt "${EXPECTED_HEALTHY_NODES}" ]]; then
  echo "[multi-node-ha] listed nodes (${#nodes[@]}) < EXPECTED_HEALTHY_NODES (${EXPECTED_HEALTHY_NODES})" >&2
  exit 1
fi

healthy_count=0
for raw in "${nodes[@]}"; do
  node="$(echo "${raw}" | xargs)"
  if [[ -z "${node}" ]]; then
    continue
  fi
  echo "[multi-node-ha] check node=${node}"
  curl -fsS "${node%/}/healthz" >/dev/null
  curl -fsS "${node%/}/v1/masque/capabilities" >/dev/null
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

echo "[multi-node-ha] check passed"

