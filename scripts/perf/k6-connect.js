import http from "k6/http";
import { check, sleep } from "k6";

export const options = {
  vus: Number(__ENV.VUS || 20),
  duration: __ENV.DURATION || "60s",
  thresholds: {
    http_req_failed: ["rate<0.05"],
    http_req_duration: ["p(95)<800"],
  },
};

const MASQUE_SERVER_URL = __ENV.MASQUE_SERVER_URL || "http://127.0.0.1:8443";
const DEVICE_TOKEN = __ENV.DEVICE_TOKEN || "";
const FINGERPRINT = __ENV.FINGERPRINT || "k6-fingerprint";
const DEST_IP = __ENV.DEST_IP || "1.1.1.1";
const DEST_PORT = Number(__ENV.DEST_PORT || 443);
const PROTOCOL = __ENV.PROTOCOL || "tcp";

export default function () {
  const payload = JSON.stringify({
    device_token: DEVICE_TOKEN,
    fingerprint: FINGERPRINT,
    destination_ip: DEST_IP,
    destination_port: DEST_PORT,
    protocol: PROTOCOL,
  });

  const res = http.post(`${MASQUE_SERVER_URL}/connect`, payload, {
    headers: { "Content-Type": "application/json" },
  });

  check(res, {
    "status is 200 or policy reject": (r) => r.status === 200 || r.status === 403 || r.status === 401,
  });

  sleep(0.2);
}
