#!/usr/bin/env bash
set -euo pipefail

ALERTMANAGER_URL="${ALERTMANAGER_URL:-http://127.0.0.1:9093}"
NOW="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"
ENDS="$(date -u -d '+10 minutes' +"%Y-%m-%dT%H:%M:%SZ" 2>/dev/null || date -u -v+10M +"%Y-%m-%dT%H:%M:%SZ")"
RUN_ID="$(date +%s)"

cat <<EOF >/tmp/afbuyers-test-alert.json
[
  {
    "labels": {
      "alertname": "AFBuyersManualTestAlert",
      "severity": "info",
      "service": "masque-server",
      "run_id": "${RUN_ID}"
    },
    "annotations": {
      "summary": "Manual test alert",
      "description": "Triggered by scripts/alerts/send-test-alert.sh"
    },
    "startsAt": "${NOW}",
    "endsAt": "${ENDS}"
  }
]
EOF

curl -sS -X POST \
  -H "Content-Type: application/json" \
  --data @/tmp/afbuyers-test-alert.json \
  "${ALERTMANAGER_URL}/api/v2/alerts"

echo
echo "[alert] test alert submitted to ${ALERTMANAGER_URL}"
