# MASQUE VPN Monorepo (Phase 2a done; Phase 2b stub done; production hardening ongoing)

> **Chinese documentation / 中文说明：** [README.zh.md](./README.zh.md) — CONNECT-IP, QUIC stub, Linux `connect-ip-tun`, env vars, and metrics (中文专页).

## Milestone status

- **Phase 1（文档化交付 M1–M4）**：已闭环（控制面 + 最小 masque 桩 + Linux 客户端 + 可观测/部署/验收材料）；详见 `docs/release-notes/m1-m4-release-notes-2026-04-25.md` 与 `docs/runbooks/m4-go-live-acceptance-report-2026-04-25.md`。
- **Phase 2a**：数据面探索与运维闭环（**服务端 TCP 探测** `POST /v1/masque/tcp-probe`、可选主监听 **HTTPS**、既有 E2E/可观测/客户端能力）；能力见 `GET /v1/masque/capabilities` 的 `tunnel.phase2a`。
- **Phase 2b（stub，本仓库已闭环）**：Linux **`connect-ip-tun`**（TUN ↔ RFC 9484 Context 0、ADDRESS/ROUTE 胶囊、**`-route split|all`**、**`-bypass-masque-host`**、**`-dns`** / **`-dns-resolvectl`** 与 **`-dns-resolvectl-fallback`**、`doctor -connect-ip`）、masque **CONNECT-IP 桩**（授权、capsule、可选 UDP/ICMP 中继、**主动 ROUTE_ADVERTISEMENT**、指标/告警/面板）；masque 在 **Linux** 上可选 **`CONNECT_IP_TUN_FORWARD`**（**每会话 host TUN 桥**，不内置 SNAT）与可选 **`CONNECT_IP_TUN_LINK_UP`**（**`ip link up`**）。与 `开发需求.md` §7.1–§7.3 对齐的是 **stub 数据面 + 客户端路由/DNS 自动化**；**非**托管 NAT 下的全协议生产 VPN。
- **Phase 2b（生产级，仍待办）**：设备 **mTLS** 与证书生命周期、控制面↔masque **双向 TLS / 非 REST 硬化**、细粒度 **RBAC**、服务端 **托管 NAT 拓扑与全 TCP·IPv6 内核路径**（`开发需求.md` §6.1–§6.3）。

### 当前开发进度（简要）

- **已完成（含 Phase 2a + Phase 2b stub）**：端到端激活/连接桩（含 Docker E2E）、`MASQUE_SERVER_URL`、`connect -dry-run`、连接重试、会话 ID、运行时状态与 `status` 摘要、`disconnect` 幂等、Prometheus（含 authorize、tcp-probe、CONNECT-IP 指标与告警）、masque **X-Request-ID** 与 `/connect` 日志、`/connect` **64KiB** 请求体上限、**tcp-probe** 与 **`doctor -tcp-probe`**、可选 **`LISTEN_TLS_*`** 主监听 TLS、客户端为每次 POST 带 **X-Request-ID**；**CONNECT-IP stub** + **`connect-ip-tun`**（重连、日志节流、会话/拨号失败上限、分段默认路由、DNS 覆盖与恢复、**resolvectl 失败回退 resolv.conf**）；masque 可选 **`CONNECT_IP_TUN_FORWARD`** / **`CONNECT_IP_TUN_LINK_UP`**（Linux 每会话 TUN 桥与可选 **`ip link up`**）。
- **未开始（生产 Phase 2b 余项）**：mTLS 与证书生命周期、控制面↔masque 双向 TLS/非 REST 硬化、细粒度 RBAC、masque **托管 SNAT/路由与全协议内核转发**。

This repository contains a closed-loop implementation and M2 upgrades:

- `control-plane/`: Laravel API (`/api/v1/...`) for provisioning, token auth, and policy management
- `masque-server/`: Go service with control-plane authorization callback and ACL enforcement
- `linux-client/`: Go CLI with `activate` / `connect` / `status` / `disconnect` / `doctor` / `version` / `config …` and route/DNS apply+restore
- `docs/adr/`: initial architecture decisions

## Step-by-step development baseline

1. Start control-plane:
   - `cd control-plane`
   - `php artisan migrate`
   - Set **`MASQUE_SERVER_URL`** in `.env` to your masque-server base URL (e.g. `http://127.0.0.1:8443`). This is what activate/config put into the client as `masque_server_url` — it must not be the Laravel `APP_URL`.
   - `php artisan serve --host=127.0.0.1 --port=8000`
2. Start masque-server:
   - `cd ../masque-server`
   - `go mod tidy`
   - `go run ./cmd/server version` (optional; prints build metadata, overridable with `-ldflags -X main.version=...`)
   - `CONTROL_PLANE_URL=http://127.0.0.1:8000 go run ./cmd/server`
   - Optional **TLS on the main TCP listener** (dev): `./scripts/dev/gen-masque-listen-tls.sh /tmp` then `LISTEN_TLS_CERT=/tmp/masque-listen.crt LISTEN_TLS_KEY=/tmp/masque-listen.key go run ./cmd/server` (set control-plane **`MASQUE_SERVER_URL`** to `https://...`; clients use `https` or `curl -k`)
   - Optional UDP **HTTP/3** health/capabilities + **CONNECT-IP stub** (self-signed TLS, dev only): `QUIC_LISTEN_ADDR=:8444` on the same process; probe with `curl --http3-only -k https://127.0.0.1:8444/healthz`. **Dev only:** `CONNECT_IP_SKIP_AUTH=1` or `MASQUE_CONNECT_IP_SKIP_AUTH=1` disables Bearer/Device-Fingerprint on CONNECT-IP (see capabilities `quic.connect_ip.dev`). **`CONNECT_IP_TUN_FORWARD`** / **`CONNECT_IP_TUN_LINK_UP`** widen the host attack surface — use only in controlled environments.
   - Linux client: `client doctor -connect-ip` dials QUIC using **`transport.http3_stub.listen_udp`** from capabilities (host taken from `masque_server_url` when listen_udp is `:port`); override with **`-connect-ip-udp 127.0.0.1:8444`** if needed.
3. Use API to create user + activation code + policy:
   - `POST /api/v1/users`
   - `POST /api/v1/devices/activation-code`
   - `POST /api/v1/users/{id}/policy` or `POST /api/v1/devices/{id}/policy`
4. Activate and connect from CLI:
   - `cd ../linux-client`
   - `go mod tidy`
   - `go run ./cmd/client version` (optional; same `-ldflags -X main.version=...` pattern as server)
   - `go run ./cmd/client activate -control-plane http://127.0.0.1:8000 -fingerprint fp-demo-001 -code XXXX-YYYY` (optional `-verify`: control plane before activate; masque `/healthz` after — masque failure only warns, config still saved)
   - `go run ./cmd/client doctor -h` (optional probes: control plane + masque `/healthz`, `-strict` requires masque URL; when capabilities advertise **TUN** per session, `doctor` also **GET `/metrics`** and expects **CONNECT-IP TUN** metric names — see [README.zh.md](./README.zh.md) doctor section)
   - `go run ./cmd/client config show` (token redacted) or `config path` / `config export` / `config import -i file [-force] [-verify]`
   - `go run ./cmd/client status -live` (local summary + `GET /api/v1/devices/self`); `status -json` / `status -json -live` for machine-readable output
   - `sudo go run ./cmd/client connect` (optional `connect -check` to require control plane `GET /api/v1/devices/self` OK before masque)
   - `go run ./cmd/client connect -dry-run` (POST `/connect` only; no `ip route` or `/etc/resolv.conf` changes — no root)
   - `go run ./cmd/client connect -connect-retries 2` (default 2 extra tries on masque **429 / 5xx** or transport errors; POST timeout 15s)
   - `sudo go run ./cmd/client disconnect`
   - `sudo go run ./cmd/client connect-ip-tun -route split -dns 1.1.1.1 -apply-routes-from-capsule` (Linux only; see [README.zh.md](./README.zh.md) for flags)

## Current scope

Implemented:

- token/JWT-like device auth (no certificate flow)
- user/device policy downlink (ACL + routes + DNS)
- server-side ACL check on requested destination
- Laravel web: registration/login, user dashboard, **admin** policy editor, audit (filters, export, archive, hash chain), one-time tokens for high-risk actions, session idle timeout, force logout
- Web login: per **email+IP** failure lockout via cache (`WEB_LOGIN_MAX_ATTEMPTS`, `WEB_LOGIN_DECAY_MINUTES`) + audit events `auth.web_login_*`
- Control plane probe: `GET /api/v1/health` (JSON) for load balancers and CLI checks
- Device introspection: `GET /api/v1/devices/self` with `Authorization: Bearer <device_token>` returns device fields + resolved policy (no secrets); stricter throttle **45/min** vs 120/min for other v1 routes
- Linux client route and DNS automation with disconnect restore; `doctor`, `config show|path|export|import` (`-verify` on import)
- server metrics endpoint for Prometheus (`/metrics`); `/healthz` JSON includes `version` string (link-time `main.version`); `/connect` returns a unique `session` id (`msq_` + 32 hex) and exposes `masque_authorize_latency_seconds` for control-plane authorize RTT; **X-Request-ID** middleware on HTTP/1.1 router (echoed on responses, logged on `/connect` outcomes — no tokens); `/connect` rejects bodies **> 64KiB** with **413** (`masque_connect_failures_total{reason="payload_too_large"}`)
- **Phase 2a:** `POST /v1/masque/tcp-probe` (JSON `device_token`, `fingerprint`, literal **`host` IP**, `port`) — authorize + ACL then **TCP dial from masque host**; metrics `masque_tcp_probe_*`; **`doctor -tcp-probe 1.1.1.1:443`**
- **Phase 2b (stub):** with **`QUIC_LISTEN_ADDR`**, the UDP HTTP/3 listener accepts **extended CONNECT** with **`:protocol connect-ip`** (RFC 9484 shape). Before **200** it calls the same control-plane **`/api/v1/server/authorize`** as TCP (unless **`CONNECT_IP_SKIP_AUTH` / `MASQUE_CONNECT_IP_SKIP_AUTH`** is set for local dev), using **`Authorization: Bearer <device_token>`** and **`Device-Fingerprint`**. Then **200** + **`Capsule-Protocol: ?1`** and **RFC 9297 + RFC 9484** parsing: **ADDRESS_ASSIGN / ADDRESS_REQUEST / ROUTE_ADVERTISEMENT** payloads are decoded; **ROUTE_ADVERTISEMENT** ranges are checked against device policy (**both ends of the inclusive range must lie in the same `allow[].cidr`**; empty ACL allows all). After **ADDRESS_REQUEST**, the stub writes **ADDRESS_ASSIGN** (documentation **192.0.2.1/32** and **2001:db8::1/128** when unspecified; explicit preferred addresses must pass the same ACL rule). **HTTP/3 datagrams (RFC 9297)** are **negotiated** on the QUIC listener (SETTINGS); inbound datagrams use **RFC 9484** framing when a leading **QUIC varint Context ID** is present (**0** = raw IP packet follows; **non-zero** is dropped unless **`CONNECT_IP_STUB_ECHO_CONTEXTS`** lists that ID for dev peel); inner **IPv4/IPv6** is checked with the same **`allow[].cidr` / `protocol` / `port`** rules as **`POST /connect`**, then **echoed** if allowed (IPv6 **Hop-by-Hop / Routing / Destination / Fragment** extension headers are skipped before reading TCP/UDP ports); **opaque** inner payloads are echoed without IP parsing. **Linux client:** `connect-ip-tun` (TUN, `-route split|all`, `-dns`, reconnect). **Masque-server:** optional IPv4 UDP/ICMP relay + proactive ROUTE push; on **Linux**, **`CONNECT_IP_TUN_FORWARD=1`** adds a **per-session host TUN** bridge for ACL-allowed IP datagrams (**`CONNECT_IP_TUN_NAME`** optional; optional **`CONNECT_IP_TUN_LINK_UP`** runs **`ip link set up`** after each TUN open). **sysctl `ip_forward`** and **iptables SNAT** remain operator-managed (not applied in-process). **`connect-ip-tun`** supports **`-dns-resolvectl-fallback`** (default on) if **`resolvectl`** fails. Metrics: **`masque_connect_ip_requests_total{result=...}`** (includes **`forbidden`**), **`masque_connect_ip_capsules_parsed_total`**, **`masque_connect_ip_capsule_parse_errors_total{cause}`**, **`masque_connect_ip_rfc9484_capsules_total{capsule}`**, **`masque_connect_ip_address_assign_writes_total`**, **`masque_connect_ip_datagrams_received_total`**, **`masque_connect_ip_datagrams_sent_total`**, **`masque_connect_ip_datagrams_dropped_total`**, **`masque_connect_ip_datagram_acl_denied_total`**, **`masque_connect_ip_datagram_unknown_context_total`**, **`masque_connect_ip_streams_active`** (gauge); with **`CONNECT_IP_TUN_FORWARD`** on Linux: **`masque_connect_ip_tun_bridge_active`**, **`masque_connect_ip_tun_open_echo_fallback_total`**, **`masque_connect_ip_tun_link_up_failures_total`** (see **`docs/runbooks/connect-ip-tun-forward-linux.md`**); Grafana **`ops/observability/grafana/dashboards/masque-overview.json`** (TUN panels); Prometheus alerts **`MasqueConnectIPTunOpenEchoFallback`** and **`MasqueConnectIPTunLinkUpFailures`** in **`ops/observability/prometheus/alerts.yml`**; **`client doctor -connect-ip`** (CONNECT + SETTINGS + **RFC 9484 Context ID 0** + **datagram echo**); optional **`doctor -connect-ip -connect-ip-rfc9484-udp`** sends a second **IPv4/UDP TEST-NET** probe (requires ACL to allow **192.0.2.1** UDP **53** when CONNECT-IP auth is on). Capabilities: **`GET /v1/masque/capabilities`** (`quic.connect_ip.rfc9484`).
- linux-client sends **X-Request-ID** (`cli_` + 16 hex) on `activate` and `connect` POSTs for log correlation with masque
- Go binaries: `client version` / `server version` subcommands for release traceability (`-ldflags -X main.version|commit|date`)
- CI workflow for Laravel + Go components (`.github/workflows/ci.yml`); Go job builds each binary with `-ldflags` (`main.version` = `ci-<run>` or tag name, `main.commit` = short SHA, `main.date` UTC) and runs `version` smoke
- observability stack via Docker Compose (`ops/observability/docker-compose.yml`)
- basic deploy and rollback scripts (`scripts/deploy/deploy.sh`, `scripts/deploy/rollback.sh`)
- Prometheus alert rules + Alertmanager baseline config

Not yet implemented (production Phase 2b and beyond):

- **Production-grade** kernel-routed MASQUE on masque-server (managed SNAT/topology, full TCP/IPv6 path — today: user-space stub + optional relays + optional **Linux** per-session **TUN** bridge only)
- mTLS certificate issuance/rotation/revocation for devices
- Fine-grained RBAC beyond the `is_admin` flag (roles/permissions matrix)
- Control-plane ↔ masque **mTLS** / gRPC-style hardened channel (beyond HTTPS authorize today)

## Client connect smoke (loopback)

With control-plane and masque-server running as above, and **`MASQUE_SERVER_URL`** matching masque:

- `./scripts/dev/e2e-client-connect-dry-run.sh` — creates a throwaway API user, issues a code, `activate`, then `connect -dry-run` using a temp config file (does not overwrite `~/.masque-client.json`).
- **Docker (same E2E, fully isolated):** from repo root, `docker compose -f docker/e2e/docker-compose.yml up --build --abort-on-container-exit --remove-orphans` (see `docker/e2e/README.md`).
- Optional isolation: `MASQUE_CLIENT_CONFIG` / `MASQUE_CLIENT_STATE` point the CLI at alternate paths (the script sets these automatically).

## M3 local observability quickstart

1. Start stack:
   - `cd ops/observability`
   - `docker compose up -d`
2. Access:
   - Prometheus: `http://127.0.0.1:9090`
   - Grafana: `http://127.0.0.1:3000` (admin/admin123)
   - Alertmanager: `http://127.0.0.1:9093`
3. Ensure `masque-server` is running on host `:8443` so Prometheus can scrape `/metrics`.

Alert rules included:
- high connect failure rate (>5% for 5m)
- high connect latency p95 (>800ms for 5m)

## Alert pipeline test

1. Start mock receiver on host:
   - `./scripts/alerts/start-mock-receiver.sh`
2. Submit a manual alert:
   - `./scripts/alerts/send-test-alert.sh`
3. Confirm delivery in receiver output logs.
   - The mock receiver prints an Alertmanager summary block (status, grouped alerts, `runbook_url`) and per-alert suggested next steps for common CONNECT-IP/TUN alerts.

Current Alertmanager webhook target is:
- `http://host.docker.internal:5001/alerts`

## Deploy and rollback scripts

- Deploy: `./scripts/deploy/deploy.sh staging` (optional Go build into `releases/<ts>/bin/`, same ldflags as CI; `BUILD_GO=0` to skip)
- Rollback: `./scripts/deploy/rollback.sh`

These are safe skeletons. Customize the service restart section for your production runtime.

## M4 production readiness

- Checklist:
  - `docs/runbooks/m4-production-readiness-checklist.md`
- Deploy manual:
  - `docs/runbooks/production-deploy-manual.md`
- Go-live acceptance report (example):
  - `docs/runbooks/m4-go-live-acceptance-report-2026-04-25.md`
- Release notes (external readable):
  - `docs/release-notes/m1-m4-release-notes-2026-04-25.md`
- Important production flag:
  - set `ALLOW_FIRST_USER_ADMIN=false` (only local bootstrap should auto-promote first admin)
- Admin session hardening:
  - set `ADMIN_SESSION_IDLE_TIMEOUT_MINUTES=30` to auto-expire idle admin sessions
- High-risk admin actions now use one-time confirmation tokens (single-use, 5-minute TTL)
- Admin can force logout a specific user (requires one-time confirmation token)
- Admin can batch force logout by scope (all users / non-admin users, excludes current operator)
- Audit logs now include tamper-evident hash chain (`prev_hash` + `entry_hash`)
- Audit chain ops:
  - `php artisan audit:backfill-chain` (seal historical records)
  - `php artisan audit:verify-chain` (integrity verification)
- Audit retention/archive:
  - `php artisan audit:archive-old --days=180` (mark old logs as archived, no physical delete)
- API idempotency (key write endpoints):
  - send `Idempotency-Key` header for `/api/v1` write operations
  - same key + same payload => replay previous result
  - same key + different payload => `409`
  - stale key cleanup: `php artisan api:cleanup-idempotency-keys --hours=72`
- Web login brute-force mitigation: set `WEB_LOGIN_MAX_ATTEMPTS` (default `5`) and `WEB_LOGIN_DECAY_MINUTES` (default `15`); use a shared cache store in production (not `array`)

## M3 staging smoke + load test

- Smoke test:
  - `./scripts/staging/smoke-test.sh`
- Full check (smoke + alert pipeline + optional k6 + report):
  - `./scripts/staging/full-check.sh`
- k6 load test (Docker-based):
  - `export DEVICE_TOKEN="<activated_device_token>"`
  - `export FINGERPRINT="<device_fingerprint>"`
  - `./scripts/perf/run-k6.sh`

Optional load variables:
- `VUS` (default `20`)
- `DURATION` (default `60s`)
- `DEST_IP` (default `1.1.1.1`)
- `DEST_PORT` (default `443`)

`full-check.sh` options:
- `RUN_K6=1` enable load test inside full check
- `CONTROL_PLANE_URL`, `MASQUE_SERVER_URL`, `PROMETHEUS_URL`, `ALERTMANAGER_URL`
