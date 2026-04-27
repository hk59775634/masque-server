#!/usr/bin/env bash
set -euo pipefail

ALERTMANAGER_URL="${ALERTMANAGER_URL:-http://127.0.0.1:9093}"
ALERTNAME="${ALERTNAME:-AFBuyersManualTestAlert}"
SEVERITY="${SEVERITY:-info}"
SERVICE="${SERVICE:-masque-server}"
SUMMARY="${SUMMARY:-Manual test alert}"
DESCRIPTION="${DESCRIPTION:-Triggered by scripts/alerts/send-test-alert.sh}"
RUNBOOK_URL="${RUNBOOK_URL:-}"
NOW="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"
ENDS="$(date -u -d '+10 minutes' +"%Y-%m-%dT%H:%M:%SZ" 2>/dev/null || date -u -v+10M +"%Y-%m-%dT%H:%M:%SZ")"
RUN_ID="$(date +%s)"
PAYLOAD_FILE="/tmp/afbuyers-test-alert.json"
DRY_RUN=0

usage() {
  cat <<'EOF'
usage: send-test-alert.sh [options]

Options:
  -u, --url URL                Alertmanager base URL (default: http://127.0.0.1:9093)
  -a, --alertname NAME         alertname label (default: AFBuyersManualTestAlert)
  -s, --severity LEVEL         severity label (default: info)
      --service NAME           service label (default: masque-server)
      --summary TEXT           annotation summary
      --description TEXT       annotation description
      --runbook-url URL        annotation runbook_url (default: inferred for known alerts)
      --dry-run                print payload JSON and skip POST
  -h, --help                   show this help

Examples:
  ./scripts/alerts/send-test-alert.sh
  ./scripts/alerts/send-test-alert.sh -a MasqueConnectIPTunLinkUpFailures -s warning
  ./scripts/alerts/send-test-alert.sh --runbook-url https://example/runbook
  ./scripts/alerts/send-test-alert.sh --dry-run -a MasqueConnectIPTunOpenEchoFallback
EOF
}

while (($# > 0)); do
  case "$1" in
    -u|--url)
      ALERTMANAGER_URL="${2:-}"; shift 2 ;;
    -a|--alertname)
      ALERTNAME="${2:-}"; shift 2 ;;
    -s|--severity)
      SEVERITY="${2:-}"; shift 2 ;;
    --service)
      SERVICE="${2:-}"; shift 2 ;;
    --summary)
      SUMMARY="${2:-}"; shift 2 ;;
    --description)
      DESCRIPTION="${2:-}"; shift 2 ;;
    --runbook-url)
      RUNBOOK_URL="${2:-}"; shift 2 ;;
    --dry-run)
      DRY_RUN=1; shift ;;
    -h|--help)
      usage; exit 0 ;;
    *)
      echo "unknown argument: $1" >&2
      usage
      exit 1 ;;
  esac
done

if [[ -z "${RUNBOOK_URL}" ]]; then
  case "${ALERTNAME}" in
    MasqueConnectIPTunOpenEchoFallback|MasqueConnectIPTunLinkUpFailures)
      RUNBOOK_URL="https://github.com/afbuyers/masque-server/blob/main/docs/runbooks/connect-ip-tun-forward-linux.md"
      ;;
    MasqueConnectIPRoutePushInvalidCIDR|MasqueConnectIPRoutePushACLDenied)
      RUNBOOK_URL="https://github.com/afbuyers/masque-server/blob/main/README.zh.md"
      ;;
    MasqueConnectFailureRateHigh|MasqueConnectLatencyP95High|MasqueAuthorizeLatencyP95High)
      RUNBOOK_URL="https://github.com/afbuyers/masque-server/blob/main/docs/runbooks/m4-production-readiness-checklist.md"
      ;;
    *)
      RUNBOOK_URL=""
      ;;
  esac
fi

ALERTNAME="${ALERTNAME}" SEVERITY="${SEVERITY}" SERVICE="${SERVICE}" RUN_ID="${RUN_ID}" \
SUMMARY="${SUMMARY}" DESCRIPTION="${DESCRIPTION}" RUNBOOK_URL="${RUNBOOK_URL}" \
NOW="${NOW}" ENDS="${ENDS}" PAYLOAD_FILE="${PAYLOAD_FILE}" \
python3 <<'PY'
import json
import os

annotations = {
    "summary": os.environ["SUMMARY"],
    "description": os.environ["DESCRIPTION"],
}
if os.environ.get("RUNBOOK_URL", "").strip():
    annotations["runbook_url"] = os.environ["RUNBOOK_URL"].strip()

payload = [{
    "labels": {
        "alertname": os.environ["ALERTNAME"],
        "severity": os.environ["SEVERITY"],
        "service": os.environ["SERVICE"],
        "run_id": os.environ["RUN_ID"],
    },
    "annotations": annotations,
    "startsAt": os.environ["NOW"],
    "endsAt": os.environ["ENDS"],
}]

with open(os.environ["PAYLOAD_FILE"], "w", encoding="utf-8") as f:
    json.dump(payload, f, ensure_ascii=False, indent=2)
PY

if [[ "${DRY_RUN}" -eq 1 ]]; then
  echo "[alert] dry-run payload (${PAYLOAD_FILE}):"
  cat "${PAYLOAD_FILE}"
  echo
  echo "[alert] dry-run only; skipped POST to ${ALERTMANAGER_URL}"
else
  curl -sS -X POST \
    -H "Content-Type: application/json" \
    --data @"${PAYLOAD_FILE}" \
    "${ALERTMANAGER_URL}/api/v2/alerts"

  echo
  echo "[alert] submitted alertname=${ALERTNAME} severity=${SEVERITY} to ${ALERTMANAGER_URL}"
  if [[ -n "${RUNBOOK_URL}" ]]; then
    echo "[alert] runbook_url=${RUNBOOK_URL}"
  fi
fi
