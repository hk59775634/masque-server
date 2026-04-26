#!/usr/bin/env python3
from http.server import BaseHTTPRequestHandler, HTTPServer
import json
from datetime import datetime


class Handler(BaseHTTPRequestHandler):
    def do_POST(self):
        length = int(self.headers.get("Content-Length", "0"))
        body = self.rfile.read(length).decode("utf-8") if length > 0 else ""
        ts = datetime.utcnow().isoformat() + "Z"
        print(f"[{ts}] POST {self.path}", flush=True)
        try:
            parsed = json.loads(body) if body else {}
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
