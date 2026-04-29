#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
CONTROL_PLANE_URL="${CONTROL_PLANE_URL:-https://www.afbuyers.com}"
MASQUE_SERVER_URL="${MASQUE_SERVER_URL:-http://127.0.0.1:8443}"
PROMETHEUS_URL="${PROMETHEUS_URL:-http://127.0.0.1:9090}"
ALERTMANAGER_URL="${ALERTMANAGER_URL:-http://127.0.0.1:9093}"
LOKI_URL="${LOKI_URL:-http://127.0.0.1:3100}"
SKIP_LOKI="${SKIP_LOKI:-0}"
GRAFANA_URL="${GRAFANA_URL:-http://127.0.0.1:3000}"
RUN_K6="${RUN_K6:-0}"
RUN_PHASE2B_KERNEL="${RUN_PHASE2B_KERNEL:-0}"
RUN_AUTHZ_HMAC_CHECK="${RUN_AUTHZ_HMAC_CHECK:-0}"
AUTHZ_HMAC_REQUIRED_EXPECTED="${AUTHZ_HMAC_REQUIRED_EXPECTED:-0}"

STAMP="$(date +%Y%m%d-%H%M%S)"
REPORT_DIR="${ROOT_DIR}/scripts/staging/reports"
REPORT_FILE="${REPORT_DIR}/full-check-${STAMP}.md"
mkdir -p "${REPORT_DIR}"

pass() { echo "- [PASS] $1" | tee -a "${REPORT_FILE}"; }
fail() { echo "- [FAIL] $1" | tee -a "${REPORT_FILE}"; }
info() { echo "- [INFO] $1" | tee -a "${REPORT_FILE}"; }

echo "# Full Check Report (${STAMP})" >"${REPORT_FILE}"
echo "" >>"${REPORT_FILE}"
echo "## Environment" >>"${REPORT_FILE}"
echo "- control_plane: ${CONTROL_PLANE_URL}" >>"${REPORT_FILE}"
echo "- masque_server: ${MASQUE_SERVER_URL}" >>"${REPORT_FILE}"
echo "- prometheus: ${PROMETHEUS_URL}" >>"${REPORT_FILE}"
echo "- alertmanager: ${ALERTMANAGER_URL}" >>"${REPORT_FILE}"
echo "- loki: ${LOKI_URL} (SKIP_LOKI=${SKIP_LOKI})" >>"${REPORT_FILE}"
echo "- grafana: ${GRAFANA_URL}" >>"${REPORT_FILE}"
echo "- run_k6: ${RUN_K6}" >>"${REPORT_FILE}"
echo "- run_phase2b_kernel: ${RUN_PHASE2B_KERNEL}" >>"${REPORT_FILE}"
echo "- run_authz_hmac_check: ${RUN_AUTHZ_HMAC_CHECK}" >>"${REPORT_FILE}"
echo "- authz_hmac_required_expected: ${AUTHZ_HMAC_REQUIRED_EXPECTED}" >>"${REPORT_FILE}"
echo "" >>"${REPORT_FILE}"
echo "## Checks" >>"${REPORT_FILE}"

SMOKE_LOG="${TMPDIR:-/tmp}/afbuyers-smoke-${STAMP}.log"
set +e
"${ROOT_DIR}/scripts/staging/smoke-test.sh" >"${SMOKE_LOG}" 2>&1
smoke_rc=$?
set -e
if [[ "${smoke_rc}" -eq 0 ]]; then
  pass "smoke-test.sh"
else
  fail "smoke-test.sh"
fi
{
  echo ""
  echo "### smoke-test.sh output (control plane probe path is visible below)"
  echo '```'
  cat "${SMOKE_LOG}"
  echo '```'
} >>"${REPORT_FILE}"

if curl -fsS "${PROMETHEUS_URL}/api/v1/targets" | python3 -c 'import json,sys; d=json.load(sys.stdin); t=d["data"]["activeTargets"]; assert any(x["labels"].get("job")=="masque-server" and x["health"]=="up" for x in t)'; then
  pass "prometheus target masque-server is up"
else
  fail "prometheus target masque-server is not up"
fi

if curl -fsS "${PROMETHEUS_URL}/api/v1/rules" | python3 -c 'import json,sys; d=json.load(sys.stdin); rules=[r["name"] for g in d["data"]["groups"] for r in g["rules"]]; assert "MasqueConnectFailureRateHigh" in rules and "MasqueConnectLatencyP95High" in rules'; then
  pass "prometheus alert rules loaded"
else
  fail "prometheus alert rules missing"
fi

if [[ "${SKIP_LOKI}" == "1" ]]; then
  info "loki /ready skipped (SKIP_LOKI=1)"
else
  code="$(curl -sS -o /dev/null -w "%{http_code}" "${LOKI_URL}/ready" || echo 000)"
  if [[ "${code}" == "200" ]]; then
    pass "loki /ready"
  elif [[ "${code}" == "503" ]]; then
    info "loki /ready -> 503 (not ready yet; common right after compose up)"
  else
    fail "loki /ready unexpected HTTP ${code}"
  fi
fi

if "${ROOT_DIR}/scripts/alerts/send-test-alert.sh" >/dev/null 2>&1; then
  pass "manual test alert submitted"
else
  fail "manual test alert submission failed"
fi

if curl -fsS "${ALERTMANAGER_URL}/api/v2/alerts" | python3 -c 'import json,sys; d=json.load(sys.stdin); assert any(a.get("labels",{}).get("alertname")=="AFBuyersManualTestAlert" for a in d)'; then
  pass "alertmanager received manual test alert"
else
  fail "alertmanager missing manual test alert"
fi

if [[ "${RUN_K6}" == "1" ]]; then
  if [[ -z "${DEVICE_TOKEN:-}" || -z "${FINGERPRINT:-}" ]]; then
    fail "k6 skipped (DEVICE_TOKEN/FINGERPRINT not provided)"
  else
    info "running k6 load test"
    if "${ROOT_DIR}/scripts/perf/run-k6.sh" >/tmp/afbuyers-k6-${STAMP}.log 2>&1; then
      pass "k6 run completed"
      echo "" >>"${REPORT_FILE}"
      echo "## k6 summary (tail)" >>"${REPORT_FILE}"
      echo '```' >>"${REPORT_FILE}"
      tail -n 30 /tmp/afbuyers-k6-${STAMP}.log >>"${REPORT_FILE}"
      echo '```' >>"${REPORT_FILE}"
    else
      fail "k6 run failed"
    fi
  fi
else
  info "k6 skipped (set RUN_K6=1 to enable)"
fi

if [[ "${RUN_PHASE2B_KERNEL}" == "1" ]]; then
  info "running phase2b kernel-forward checks"
  if "${ROOT_DIR}/scripts/staging/phase2b-kernel-check.sh" >>"${REPORT_FILE}" 2>&1; then
    pass "phase2b-kernel-check.sh"
  else
    fail "phase2b-kernel-check.sh"
  fi
else
  info "phase2b kernel checks skipped (set RUN_PHASE2B_KERNEL=1 to enable)"
fi

if [[ "${RUN_AUTHZ_HMAC_CHECK}" == "1" ]]; then
  info "running authorize HMAC checks"
  if "${ROOT_DIR}/scripts/staging/authz-hmac-check.sh" >>"${REPORT_FILE}" 2>&1; then
    pass "authz-hmac-check.sh"
  else
    fail "authz-hmac-check.sh"
  fi
else
  info "authorize HMAC checks skipped (set RUN_AUTHZ_HMAC_CHECK=1 to enable)"
fi

echo "" >>"${REPORT_FILE}"
echo "## Quick Queries" >>"${REPORT_FILE}"
echo "- ${PROMETHEUS_URL}/graph?g0.expr=rate(masque_connect_requests_total%5B5m%5D)&g0.tab=0" >>"${REPORT_FILE}"
echo "- ${PROMETHEUS_URL}/graph?g0.expr=rate(masque_connect_failures_total%5B5m%5D)&g0.tab=0" >>"${REPORT_FILE}"
echo "- ${PROMETHEUS_URL}/graph?g0.expr=histogram_quantile(0.95%2Csum(rate(masque_authorize_latency_seconds_bucket%5B5m%5D))by(le))&g0.tab=0" >>"${REPORT_FILE}"
echo "- ${ALERTMANAGER_URL}/#/alerts" >>"${REPORT_FILE}"
echo "- ${ALERTMANAGER_URL}/#/silences" >>"${REPORT_FILE}"
echo "- ${LOKI_URL}/ready" >>"${REPORT_FILE}"
echo "- ${GRAFANA_URL}/d/afbuyers-loki-cheatsheet/loki-logql-cheatsheet" >>"${REPORT_FILE}"

echo "[full-check] report generated: ${REPORT_FILE}"
