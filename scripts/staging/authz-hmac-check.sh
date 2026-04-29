#!/usr/bin/env bash
set -euo pipefail

CONTROL_PLANE_URL="${CONTROL_PLANE_URL:-https://www.afbuyers.com}"
AUTHZ_HMAC_SECRET="${AUTHZ_HMAC_SECRET:-}"
AUTHZ_HMAC_REQUIRED_EXPECTED="${AUTHZ_HMAC_REQUIRED_EXPECTED:-0}"

if ! command -v jq >/dev/null 2>&1; then
  echo "[authz-hmac] jq is required" >&2
  exit 1
fi

cp="${CONTROL_PLANE_URL%/}"
echo "[authz-hmac] control-plane=${cp}"
echo "[authz-hmac] required_expected=${AUTHZ_HMAC_REQUIRED_EXPECTED}"

email="authz-hmac-$(date +%s)-$RANDOM@example.test"
fp="fp-authz-hmac-$(date +%s)-$RANDOM"

user_json="$(curl -fsS -X POST "${cp}/api/v1/users" -H 'Content-Type: application/json' -d "{\"name\":\"HMAC Gate\",\"email\":\"${email}\",\"password\":\"password123\"}")"
user_id="$(echo "${user_json}" | jq -r '.user.id')"

code_json="$(curl -fsS -X POST "${cp}/api/v1/devices/activation-code" -H 'Content-Type: application/json' -d "{\"user_id\":${user_id},\"device_name\":\"hmac-check\",\"fingerprint\":\"${fp}\"}")"
raw_code="$(echo "${code_json}" | jq -r '.activation_code')"

act_json="$(curl -fsS -X POST "${cp}/api/v1/activate" -H 'Content-Type: application/json' -d "{\"activation_code\":\"${raw_code}\",\"fingerprint\":\"${fp}\"}")"
device_token="$(echo "${act_json}" | jq -r '.device_token')"

payload="$(jq -cn --arg tok "${device_token}" --arg fp "${fp}" '{device_token:$tok,fingerprint:$fp}')"

unsigned_code="$(curl -sS -o /tmp/authz-hmac-unsigned.$$ -w "%{http_code}" -X POST "${cp}/api/v1/server/authorize" -H 'Content-Type: application/json' --data "${payload}")"
echo "[authz-hmac] unsigned /server/authorize HTTP ${unsigned_code}"

if [[ "${AUTHZ_HMAC_REQUIRED_EXPECTED}" == "1" ]]; then
  if [[ "${unsigned_code}" != "401" ]]; then
    echo "[authz-hmac] expected unsigned request to fail with 401 when required=1" >&2
    exit 1
  fi
fi

if [[ -z "${AUTHZ_HMAC_SECRET}" ]]; then
  if [[ "${AUTHZ_HMAC_REQUIRED_EXPECTED}" == "1" ]]; then
    echo "[authz-hmac] AUTHZ_HMAC_SECRET is required when AUTHZ_HMAC_REQUIRED_EXPECTED=1" >&2
    exit 1
  fi
  echo "[authz-hmac] no AUTHZ_HMAC_SECRET provided; skipping signed-path check"
  exit 0
fi

readarray -t sig_parts < <(python3 - <<'PY' "${payload}" "${AUTHZ_HMAC_SECRET}"
import hashlib, hmac, json, sys, time
payload = json.loads(sys.argv[1])
secret = sys.argv[2].encode()
body = json.dumps(payload, separators=(",", ":"), ensure_ascii=False)
ts = str(int(time.time()))
body_hash = hashlib.sha256(body.encode()).hexdigest()
msg = "\n".join(["POST", "/api/v1/server/authorize", ts, body_hash]).encode()
sig = hmac.new(secret, msg, hashlib.sha256).hexdigest()
print(body)
print(ts)
print(sig)
PY
)

signed_body="${sig_parts[0]}"
signed_ts="${sig_parts[1]}"
signed_sig="${sig_parts[2]}"

signed_code="$(curl -sS -o /tmp/authz-hmac-signed.$$ -w "%{http_code}" -X POST "${cp}/api/v1/server/authorize" \
  -H 'Content-Type: application/json' \
  -H "X-Masque-Authz-Timestamp: ${signed_ts}" \
  -H "X-Masque-Authz-Signature: ${signed_sig}" \
  --data "${signed_body}")"
echo "[authz-hmac] signed /server/authorize HTTP ${signed_code}"

if [[ "${signed_code}" != "200" ]]; then
  echo "[authz-hmac] expected signed request to succeed with 200" >&2
  exit 1
fi

echo "[authz-hmac] check passed"

