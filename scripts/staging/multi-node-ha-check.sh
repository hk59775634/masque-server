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
matrix_json='[]'
normalized_nodes_json='[]'
for raw in "${nodes[@]}"; do
  node="$(echo "${raw}" | xargs)"
  if [[ -z "${node}" ]]; then
    continue
  fi
  echo "[multi-node-ha] check node=${node}"
  curl -fsS "${node%/}/healthz" >/dev/null
  caps="$(curl -fsS "${node%/}/v1/masque/capabilities")"
  matrix_json="$(python3 - <<'PY' "${matrix_json}" "${node}" "${caps}"
import json, sys
arr = json.loads(sys.argv[1])
node = sys.argv[2]
cap = json.loads(sys.argv[3])
arr.append({"kind": "node", "url": node, "cap": cap})
print(json.dumps(arr, separators=(",", ":")))
PY
)"
  normalized_nodes_json="$(python3 - <<'PY' "${normalized_nodes_json}" "${node}"
import json, sys
from urllib.parse import urlparse
arr = json.loads(sys.argv[1])
u = urlparse(sys.argv[2])
host = (u.hostname or "").strip().lower()
port = u.port or (443 if u.scheme == "https" else 80)
arr.append(f"{host}:{port}")
print(json.dumps(arr, separators=(",", ":")))
PY
)"
  if [[ -z "${baseline_caps}" ]]; then
    baseline_caps="${caps}"
  else
    python3 - <<'PY' "${baseline_caps}" "${caps}" "${node}"
import json, sys
base = json.loads(sys.argv[1])
cur = json.loads(sys.argv[2])
node = sys.argv[3]

def profile(cap):
    ci = (((cap.get("tunnel") or {}).get("quic") or {}).get("connect_ip") or {})
    dg = (ci.get("http3_datagrams") or {})
    return {
        "tun_linux_per_session": bool(dg.get("tun_linux_per_session", False)),
        "tun_linux_managed_nat": bool(dg.get("tun_linux_managed_nat", False)),
        "tun_linux_shared": bool(dg.get("tun_linux_shared", False)),
        "tun_linux_managed_nat_backend": str(dg.get("tun_linux_managed_nat_backend", "")),
        "ip_forward": str(dg.get("ip_forward", "")),
    }

bp = profile(base)
cp = profile(cur)
if bp != cp:
    raise SystemExit(f"node capability profile mismatch: node={node} baseline={bp} current={cp}")
PY
  fi
  healthy_count=$((healthy_count + 1))
done

if [[ "${healthy_count}" -lt "${EXPECTED_HEALTHY_NODES}" ]]; then
  echo "[multi-node-ha] healthy node checks (${healthy_count}) < expected (${EXPECTED_HEALTHY_NODES})" >&2
  exit 1
fi

targets_raw="$(curl -fsS "${PROMETHEUS_URL}/api/v1/targets")"
python3 - <<'PY' "${targets_raw}" "${EXPECTED_HEALTHY_NODES}" "${normalized_nodes_json}"
import json, sys
d = json.loads(sys.argv[1])
expected = int(sys.argv[2])
expected_instances = set(json.loads(sys.argv[3]))
targets = d.get("data", {}).get("activeTargets", [])
masque_targets = [t for t in targets if t.get("labels", {}).get("job") == "masque-server"]
healthy = [t for t in masque_targets if t.get("health") == "up"]
if len(healthy) < expected:
    raise SystemExit(f"prometheus healthy masque-server targets={len(healthy)} < expected={expected}")
print(f"[multi-node-ha] prometheus healthy masque-server targets={len(healthy)}")

healthy_instances = set()
for t in healthy:
    inst = str((t.get("labels", {}) or {}).get("instance", "")).strip().lower()
    if inst:
        healthy_instances.add(inst)
missing = sorted(expected_instances - healthy_instances)
if missing:
    raise SystemExit(f"expected node instances not healthy in prometheus: {missing}")
print(f"[multi-node-ha] expected instances healthy={sorted(expected_instances)}")

print("[multi-node-ha] prometheus target detail markdown follows")
print("### Prometheus Target Detail (masque-server)")
print("")
print("| instance | health | scrape_url | last_error |")
print("|---|---|---|---|")
for t in masque_targets:
    labels = t.get("labels", {})
    instance = labels.get("instance", "-")
    health = t.get("health", "-")
    scrape = t.get("scrapeUrl", "-")
    err = t.get("lastError", "") or "-"
    # Keep markdown table single-line friendly.
    err = str(err).replace("|", "/").replace("\n", " ").strip() or "-"
    print(f"| {instance} | {health} | {scrape} | {err} |")
PY

if [[ -n "${MASQUE_LB_URL}" ]]; then
  lb="${MASQUE_LB_URL%/}"
  echo "[multi-node-ha] check load-balancer endpoint=${lb}"
  curl -fsS "${lb}/healthz" >/dev/null
  lb_caps="$(curl -fsS "${lb}/v1/masque/capabilities")"
  matrix_json="$(python3 - <<'PY' "${matrix_json}" "${lb}" "${lb_caps}"
import json, sys
arr = json.loads(sys.argv[1])
lb = sys.argv[2]
cap = json.loads(sys.argv[3])
arr.append({"kind": "lb", "url": lb, "cap": cap})
print(json.dumps(arr, separators=(",", ":")))
PY
)"
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

python3 - <<'PY' "${matrix_json}" "${EXPECTED_HEALTHY_NODES}"
import json, sys
rows = json.loads(sys.argv[1])
expected = int(sys.argv[2])

def extract(cap):
    ci = (((cap.get("tunnel") or {}).get("quic") or {}).get("connect_ip") or {})
    dg = (ci.get("http3_datagrams") or {})
    return {
        "tun_linux_per_session": "yes" if dg.get("tun_linux_per_session", False) else "no",
        "tun_linux_managed_nat": "yes" if dg.get("tun_linux_managed_nat", False) else "no",
        "tun_linux_shared": "yes" if dg.get("tun_linux_shared", False) else "no",
        "managed_nat_backend": str(dg.get("tun_linux_managed_nat_backend", "")) or "-",
        "ip_forward": str(dg.get("ip_forward", "")) or "-",
    }

print("[multi-node-ha] summary")
print("[multi-node-ha] expected_healthy_nodes=%d listed_endpoints=%d" % (expected, len(rows)))
print("[multi-node-ha] capability matrix markdown follows")
print("### Multi-node HA Capability Matrix")
print("")
print("| Endpoint | Type | tun_per_session | managed_nat | shared_tun | nat_backend | ip_forward |")
print("|---|---|---|---|---|---|---|")
for row in rows:
    p = extract(row["cap"])
    print(
        f"| {row['url']} | {row['kind']} | {p['tun_linux_per_session']} | {p['tun_linux_managed_nat']} | {p['tun_linux_shared']} | {p['managed_nat_backend']} | {p['ip_forward']} |"
    )
PY

echo "[multi-node-ha] check passed"

