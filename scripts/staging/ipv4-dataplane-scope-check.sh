#!/usr/bin/env bash
set -euo pipefail

MASQUE_SERVER_URL="${MASQUE_SERVER_URL:-http://127.0.0.1:8443}"
echo "[ipv4-scope] masque_server=${MASQUE_SERVER_URL}"

caps_raw="$(curl -fsS "${MASQUE_SERVER_URL%/}/v1/masque/capabilities")"
python3 - <<'PY' "${caps_raw}"
import json, sys

cap = json.loads(sys.argv[1])
ci = ((cap.get("tunnel") or {}).get("quic") or {}).get("connect_ip") or {})
not_impl = ((ci.get("rfc9484") or {}).get("not_implemented") or [])
must_keep = "CONNECT-IP TCP or IPv6 datagram relay"
if must_keep not in not_impl:
    raise SystemExit(f"missing IPv4-only scope guard in rfc9484.not_implemented: {must_keep!r}")
print("[ipv4-scope] rfc9484.not_implemented contains expected IPv4-only guard")
print("[ipv4-scope] check passed")
PY
