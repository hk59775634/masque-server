#!/usr/bin/env python3
from http.server import BaseHTTPRequestHandler, HTTPServer
import json
from datetime import datetime


SUGGESTIONS = {
    "MasqueConnectIPTunOpenEchoFallback": [
        "Check /dev/net/tun exists and masque-server has CAP_NET_ADMIN/root.",
        "Verify CONNECT_IP_TUN_FORWARD=1 and CONNECT_IP_TUN_NAME (if set) are valid.",
        "Inspect masque logs around 'tun forward unavailable' for exact errno.",
    ],
    "MasqueConnectIPTunLinkUpFailures": [
        "Verify ip(8) is present in PATH for the masque-server process.",
        "Check CAP_NET_ADMIN/root; run manual `ip link set dev <tun> up` on host.",
        "Confirm CONNECT_IP_TUN_LINK_UP is intended; disable if host manages link state.",
    ],
    "MasqueConnectIPRoutePushInvalidCIDR": [
        "Validate CONNECT_IP_ROUTE_ADV_CIDR format and mask.",
        "Confirm deployed env matches expected CIDR value (no trailing spaces).",
    ],
    "MasqueConnectIPRoutePushACLDenied": [
        "Compare CONNECT_IP_ROUTE_ADV_CIDR range against device ACL allow.cidr.",
        "Ensure both route start/end fall within one ACL rule (server policy rule).",
    ],
}


def first_non_empty(*vals):
    for v in vals:
        if isinstance(v, str) and v.strip():
            return v.strip()
    return ""


def print_alertmanager_summary(payload):
    if not isinstance(payload, dict) or "alerts" not in payload:
        return False
    alerts = payload.get("alerts") or []
    common_labels = payload.get("commonLabels") or {}
    common_annotations = payload.get("commonAnnotations") or {}
    group_labels = payload.get("groupLabels") or {}
    external_url = payload.get("externalURL", "")
    receiver = payload.get("receiver", "")
    status = payload.get("status", "")
    runbook = first_non_empty(
        common_annotations.get("runbook_url", ""),
        *((a.get("annotations") or {}).get("runbook_url", "") for a in alerts if isinstance(a, dict)),
    )

    print("alertmanager-webhook-summary", flush=True)
    print(f"  status: {status}", flush=True)
    print(f"  receiver: {receiver}", flush=True)
    print(f"  alert_count: {len(alerts)}", flush=True)
    if group_labels:
        print(f"  group_labels: {group_labels}", flush=True)
    if common_labels:
        print(f"  common_labels: {common_labels}", flush=True)
    if runbook:
        print(f"  runbook_url: {runbook}", flush=True)
    if external_url:
        print(f"  source: {external_url}", flush=True)

    for idx, alert in enumerate(alerts, start=1):
        if not isinstance(alert, dict):
            continue
        labels = alert.get("labels") or {}
        annotations = alert.get("annotations") or {}
        name = labels.get("alertname", "unknown")
        sev = labels.get("severity", "")
        summary = first_non_empty(annotations.get("summary", ""), annotations.get("description", ""))
        starts_at = alert.get("startsAt", "")
        ends_at = alert.get("endsAt", "")
        print(f"  [{idx}] {name} severity={sev}", flush=True)
        if summary:
            print(f"      summary: {summary}", flush=True)
        if starts_at:
            print(f"      starts_at: {starts_at}", flush=True)
        if ends_at:
            print(f"      ends_at: {ends_at}", flush=True)
        suggested = SUGGESTIONS.get(name)
        if suggested:
            print("      suggested_next_steps:", flush=True)
            for step in suggested:
                print(f"        - {step}", flush=True)
    return True


class Handler(BaseHTTPRequestHandler):
    def do_POST(self):
        length = int(self.headers.get("Content-Length", "0"))
        body = self.rfile.read(length).decode("utf-8") if length > 0 else ""
        ts = datetime.utcnow().isoformat() + "Z"
        print(f"[{ts}] POST {self.path}", flush=True)
        try:
            parsed = json.loads(body) if body else {}
            handled = print_alertmanager_summary(parsed)
            if handled:
                print("-" * 80, flush=True)
            print(json.dumps(parsed, ensure_ascii=False, indent=2), flush=True)
        except Exception:
            print(body, flush=True)
        print("-" * 80, flush=True)
        self.send_response(200)
        self.end_headers()
        self.wfile.write(b"ok")

    def log_message(self, _format, *_args):
        return


if __name__ == "__main__":
    server = HTTPServer(("0.0.0.0", 5001), Handler)
    print("Mock alert receiver listening on http://0.0.0.0:5001/alerts", flush=True)
    server.serve_forever()
