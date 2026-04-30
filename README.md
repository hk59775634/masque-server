# MASQUE VPN Monorepo (Phase 2a done; Phase 2b stub done; production hardening ongoing)

> **Chinese documentation / 中文说明：** [README.zh.md](./README.zh.md) — CONNECT-IP, QUIC stub, Linux `connect-ip-tun`, env vars, and metrics (中文专页).

## Milestone status

- **Phase 1（文档化交付 M1–M4）**：已闭环（控制面 + 最小 masque 桩 + Linux 客户端 + 可观测/部署/验收材料）；详见 `docs/release-notes/m1-m4-release-notes-2026-04-25.md` 与 `docs/runbooks/m4-go-live-acceptance-report-2026-04-25.md`。
- **Phase 2a**：数据面探索与运维闭环（**服务端 TCP 探测** `POST /v1/masque/tcp-probe`、可选主监听 **HTTPS**、既有 E2E/可观测/客户端能力）；能力见 `GET /v1/masque/capabilities` 的 `tunnel.phase2a`。
- **Phase 2b（stub，本仓库已闭环）**：Linux **`connect-ip-tun`**（TUN ↔ RFC 9484 Context 0、ADDRESS/ROUTE 胶囊、**`-route split|all`**、**`-bypass-masque-host`**、**`-dns`** / **`-dns-resolvectl`** 与 **`-dns-resolvectl-fallback`**、`doctor -connect-ip`）、masque **CONNECT-IP 桩**（授权、capsule、可选 UDP/ICMP 中继、**主动 ROUTE_ADVERTISEMENT**、指标/告警/面板）；masque 在 **Linux** 上可选 **`CONNECT_IP_TUN_FORWARD`**（**每会话 host TUN 桥**，不内置 SNAT）与可选 **`CONNECT_IP_TUN_LINK_UP`**（**`ip link up`**）。与 `开发需求.md` §7.1–§7.3 对齐的是 **stub 数据面 + 客户端路由/DNS 自动化**；**非**托管 NAT 下的全协议生产 VPN。
- **Phase 2b（Linux 数据面 P0，主线已落地）**：**`CONNECT_IP_TUN_MANAGED_NAT`** 下 **nftables 优先**与可配置 **iptables 回退**、部署前置 **`scripts/deploy/dataplane-preflight.sh`**、托管 NAT / 共享绑定相关指标与告警（含 nft fallback、active reassign）、运维 **`scripts/vpn-nat-backend-fault-injection.sh`** 与 Actions **`VPN NAT fault-injection script`**（语法 + `--dry-run` smoke）。详见 **`docs/runbooks/connect-ip-tun-forward-linux.md`**。
- **Phase 2b（生产级，仍待办）**：细粒度 **RBAC**、控制面↔masque **通道硬化**（HTTPS 强化或后续 gRPC over **publicly trusted TLS**）、**全 TCP / 内核路径**、**multi-node HA** 等。**Out of scope (documented in `开发需求.md` §2.3):** device **mTLS / client-cert identity**, org-wide **private CA issuance & revocation** for endpoints; **IPv6 dataplane** is not a near-term deliverable.

### 当前开发进度（简要）

- **已完成（含 Phase 2a + Phase 2b stub + Linux 数据面 P0）**：端到端激活/连接桩（含 Docker E2E）、`MASQUE_SERVER_URL`、`connect -dry-run`、连接重试、会话 ID、运行时状态与 `status` 摘要、`disconnect` 幂等、Prometheus（含 authorize、tcp-probe、CONNECT-IP 指标与告警）、masque **X-Request-ID** 与 `/connect` 日志、`/connect` **64KiB** 请求体上限、**tcp-probe** 与 **`doctor -tcp-probe`**、可选 **`LISTEN_TLS_*`** 主监听 TLS、客户端为每次 POST 带 **X-Request-ID**；**CONNECT-IP stub** + **`connect-ip-tun`**（重连、日志节流、会话/拨号失败上限、分段默认路由、DNS 覆盖与恢复、**resolvectl 失败回退 resolv.conf**）；masque 可选 **`CONNECT_IP_TUN_FORWARD`** / **`CONNECT_IP_TUN_LINK_UP`** / **`CONNECT_IP_TUN_MANAGED_NAT`**（Linux：每会话或共享 TUN、托管 NAT **nft/iptables**、deploy **preflight**、故障注入与 CI smoke）。
- **进行中（生产 Phase 2b 余项）**：细粒度 **RBAC**（基础表/权限中间件已落地，仍需管理界面与授权策略完善）、控制面↔masque **通道硬化**（HMAC 签名已支持，待强制化 rollout）、相对现状仍缺的 **全 TCP 内核路径 / 多节点 HA**。**不规划**：设备 **mTLS**、组织级**自建 CA 设备证书**；**IPv6 数据面**不在短期范围（`开发需求.md` §2.3）。

This repository contains a closed-loop implementation and M2 upgrades:

- `control-plane/`: Laravel API (`/api/v1/...`) for provisioning, token auth, and policy management
- `masque-server/`: Go service with control-plane authorization callback and ACL enforcement
- `linux-client/`: Go CLI with `activate` / **`quick-login`** / `connect` / `status` / `disconnect` / `doctor` / `version` / `config …` and route/DNS apply+restore
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
  - Optional control-plane authorize hardening (shared-secret HMAC on `POST /api/v1/server/authorize`): set the same secret on both sides:
    - masque-server: `CONTROL_PLANE_AUTHZ_HMAC_SECRET=...`
    - control-plane: `MASQUE_AUTHORIZE_HMAC_SECRET=...` (and optionally `MASQUE_AUTHORIZE_HMAC_REQUIRED=true`, `MASQUE_AUTHORIZE_HMAC_WINDOW_SECONDS=300`)
   - Optional **TLS on the main TCP listener** (dev): `./scripts/dev/gen-masque-listen-tls.sh /tmp` then `LISTEN_TLS_CERT=/tmp/masque-listen.crt LISTEN_TLS_KEY=/tmp/masque-listen.key go run ./cmd/server` (set control-plane **`MASQUE_SERVER_URL`** to `https://...`; clients use `https` or `curl -k`)
   - Optional UDP **HTTP/3** health/capabilities + **CONNECT-IP stub** (self-signed TLS, dev only): `QUIC_LISTEN_ADDR=:8444` on the same process; probe with `curl --http3-only -k https://127.0.0.1:8444/healthz`. **Dev only:** `CONNECT_IP_SKIP_AUTH=1` or `MASQUE_CONNECT_IP_SKIP_AUTH=1` disables Bearer/Device-Fingerprint on CONNECT-IP (see capabilities `quic.connect_ip.dev`). **`CONNECT_IP_TUN_FORWARD`** / **`CONNECT_IP_TUN_SHARED`** / **`CONNECT_IP_TUN_LINK_UP`** / **`CONNECT_IP_TUN_MANAGED_NAT`** widen the host attack surface — use only in controlled environments.
   - Linux client: `client doctor -connect-ip` dials QUIC using **`transport.http3_stub.listen_udp`** from capabilities (host taken from `masque_server_url` when listen_udp is `:port`); override with **`-connect-ip-udp 127.0.0.1:8444`** if needed.
3. Use API to create user + activation code + policy:
   - `POST /api/v1/users`
   - `POST /api/v1/devices/activation-code`
   - `POST /api/v1/devices/activation-code-with-credentials` (email + password + `fingerprint` + optional `device_name`; returns `activation_code` for the same `POST /api/v1/activate` flow; throttled)
   - **`POST /api/v1/devices/bootstrap`** (email + password + `fingerprint` + optional `device_name`; **one call** returns the same JSON as `POST /api/v1/activate` — `device_token` + `config`; throttled; optional `Idempotency-Key`)
   - `POST /api/v1/users/{id}/policy` or `POST /api/v1/devices/{id}/policy`
   - `POST /api/v1/device/token/rotate` (Bearer token + `fingerprint`; rotate device token)
   - `POST /api/v1/device/token/revoke` (Bearer token; revoke current device token immediately)
4. Activate and connect from CLI:
   - `cd ../linux-client`
   - `go mod tidy`
   - `go run ./cmd/client version` (optional; same `-ldflags -X main.version=...` pattern as server)
   - **Simplest enroll:** `go run ./cmd/client quick-login -control-plane https://your-cp.example.com -email you@example.com` with password in **`MASQUE_PASSWORD`** (or `-password`, not recommended on shared hosts). Creates/reads device fingerprint under **`~/.config/masque-linux-client/device-fingerprint`**, calls **`POST /api/v1/devices/bootstrap`**, writes **`~/.masque-client.json`**. Then run `connect` / `connect-ip-tun` as before.
   - Manual two-step: `go run ./cmd/client activate -control-plane http://127.0.0.1:8000 -fingerprint fp-demo-001 -code XXXX-YYYY` (optional `-verify`: control plane before activate; masque `/healthz` after — masque failure only warns, config still saved)
   - One-liner helper: `./scripts/masque-quick-connect.sh` — prompts for control-plane URL (if unset), email/password on first enroll (**single bootstrap API call**), then **`sudo connect-ip-tun`** on **`tun0`** with **split routes**, **`-apply-routes-from-capsule`**, and **DNS** (from saved config or `1.1.1.1,8.8.8.8`). If `CONNECT_IP_UDP` is unset, it infers **`${masque_host}:8444`** from `~/.masque-client.json` (`AUTO_CONNECT_IP_UDP_PORT`, `AUTO_CONNECT_IP_UDP=0` to disable). Masque must still listen UDP (**`QUIC_LISTEN_ADDR`**). Legacy HTTP-only: `LEGACY_CONNECT=1` + `CONNECT_MODE=dry-run|real`.
   - `go run ./cmd/client doctor -h` (optional probes: control plane + masque `/healthz`, `-strict` requires masque URL; when capabilities advertise **TUN** per session, `doctor` also **GET `/metrics`** and expects **CONNECT-IP TUN** metric names — see [README.zh.md](./README.zh.md) doctor section)
   - `go run ./cmd/client config show` (token redacted) or `config path` / `config export` / `config import -i file [-force] [-verify]`
   - `go run ./cmd/client status -live` (local summary + `GET /api/v1/devices/self`); `status -json` / `status -json -live` for machine-readable output
   - `sudo go run ./cmd/client connect` (optional `connect -check` to require control plane `GET /api/v1/devices/self` OK before masque)
   - `go run ./cmd/client connect [-masque-server URL] ...` (override saved `masque_server_url` for this run if config still points at `127.0.0.1` from an old activate)
   - `go run ./cmd/client connect -route-dev tun0` (Linux only: apply policy routes via **TUN/other ifname**; **never** `lo` — older builds wrongly used `lo` for split defaults and broke networking)
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
- **Phase 2b (stub):** with **`QUIC_LISTEN_ADDR`**, the UDP HTTP/3 listener accepts **extended CONNECT** with **`:protocol connect-ip`** (RFC 9484 shape). Before **200** it calls the same control-plane **`/api/v1/server/authorize`** as TCP (unless **`CONNECT_IP_SKIP_AUTH` / `MASQUE_CONNECT_IP_SKIP_AUTH`** is set for local dev), using **`Authorization: Bearer <device_token>`** and **`Device-Fingerprint`**. Then **200** + **`Capsule-Protocol: ?1`** and **RFC 9297 + RFC 9484** parsing: **ADDRESS_ASSIGN / ADDRESS_REQUEST / ROUTE_ADVERTISEMENT** payloads are decoded; **ROUTE_ADVERTISEMENT** ranges are checked against device policy (**both ends of the inclusive range must lie in the same `allow[].cidr`**; empty ACL allows all). After **ADDRESS_REQUEST**, the stub writes **ADDRESS_ASSIGN** (documentation **192.0.2.1/32** and **2001:db8::1/128** when unspecified; explicit preferred addresses must pass the same ACL rule). **HTTP/3 datagrams (RFC 9297)** are **negotiated** on the QUIC listener (SETTINGS); inbound datagrams use **RFC 9484** framing when a leading **QUIC varint Context ID** is present (**0** = raw IP packet follows; **non-zero** is dropped unless **`CONNECT_IP_STUB_ECHO_CONTEXTS`** lists that ID for dev peel); inner **IPv4/IPv6** is checked with the same **`allow[].cidr` / `protocol` / `port`** rules as **`POST /connect`**, then **echoed** if allowed (IPv6 **Hop-by-Hop / Routing / Destination / Fragment** extension headers are skipped before reading TCP/UDP ports); **opaque** inner payloads are echoed without IP parsing. **Linux client:** `connect-ip-tun` (TUN, `-route split|all`, `-dns`, reconnect). **Masque-server:** optional IPv4 UDP/ICMP relay + proactive ROUTE push; on **Linux**, **`CONNECT_IP_TUN_FORWARD=1`** enables kernel TUN forwarding with optional **`CONNECT_IP_TUN_SHARED=1`** (shared TUN + destination-IP demux across streams), **`CONNECT_IP_TUN_SHARED_BINDING_TTL`** (default `5m`), **`CONNECT_IP_TUN_LINK_UP`**, and **`CONNECT_IP_TUN_MANAGED_NAT`** (`ip_forward`/iptables automation with `CONNECT_IP_TUN_EGRESS_IFACE` and optional `CONNECT_IP_TUN_ADDR_CIDR`). **`connect-ip-tun`** supports **`-dns-resolvectl-fallback`** (default on) if **`resolvectl`** fails. Metrics: **`masque_connect_ip_requests_total{result=...}`** (includes **`forbidden`**), **`masque_connect_ip_capsules_parsed_total`**, **`masque_connect_ip_capsule_parse_errors_total{cause}`**, **`masque_connect_ip_rfc9484_capsules_total{capsule}`**, **`masque_connect_ip_address_assign_writes_total`**, **`masque_connect_ip_datagrams_received_total`**, **`masque_connect_ip_datagrams_sent_total`**, **`masque_connect_ip_datagrams_dropped_total`**, **`masque_connect_ip_datagram_acl_denied_total`**, **`masque_connect_ip_datagram_unknown_context_total`**, **`masque_connect_ip_streams_active`** (gauge); with TUN forwarding: **`masque_connect_ip_tun_bridge_active`**, **`masque_connect_ip_tun_open_echo_fallback_total`**, **`masque_connect_ip_tun_link_up_failures_total`**, **`masque_connect_ip_tun_managed_nat_apply_total{result}`**, **`masque_connect_ip_tun_shared_binding_conflicts_total`**, **`masque_connect_ip_tun_shared_binding_stale_evictions_total`** (see **`docs/runbooks/connect-ip-tun-forward-linux.md`**); Grafana **`ops/observability/grafana/dashboards/masque-overview.json`** (TUN panels); Prometheus alerts **`MasqueConnectIPTunOpenEchoFallback`**, **`MasqueConnectIPTunLinkUpFailures`**, **`MasqueConnectIPTunManagedNATApplyErrors`**, and **`MasqueConnectIPTunSharedBindingConflictsHigh`** in **`ops/observability/prometheus/alerts.yml`**; **`client doctor -connect-ip`** (CONNECT + SETTINGS + **RFC 9484 Context ID 0** + **datagram echo**); optional **`doctor -connect-ip -connect-ip-rfc9484-udp`** sends a second **IPv4/UDP TEST-NET** probe (requires ACL to allow **192.0.2.1** UDP **53** when CONNECT-IP auth is on). Capabilities: **`GET /v1/masque/capabilities`** (`quic.connect_ip.rfc9484`).
- linux-client sends **X-Request-ID** (`cli_` + 16 hex) on `activate` and `connect` POSTs for log correlation with masque
- Go binaries: `client version` / `server version` subcommands for release traceability (`-ldflags -X main.version|commit|date`)
- CI workflow for Laravel + Go components (`.github/workflows/ci.yml`); Go job builds each binary with `-ldflags` (`main.version` = `ci-<run>` or tag name, `main.commit` = short SHA, `main.date` UTC) and runs `version` smoke
- observability stack via Docker Compose (`ops/observability/docker-compose.yml`)
- basic deploy and rollback scripts (`scripts/deploy/deploy.sh`, `scripts/deploy/rollback.sh`)
- Prometheus alert rules + Alertmanager baseline config

Not yet implemented (production Phase 2b and beyond):

- **Production-grade** kernel path on masque-server (full **TCP** on inner traffic where required, **multi-node** — today: user-space echo/relay + optional **Linux** TUN + **managed NAT** automation; **IPv6 dataplane not planned short-term**, see `开发需求.md` §2.3)
- Fine-grained RBAC beyond the `is_admin` flag (roles/permissions matrix)
- Control-plane ↔ masque hardened channel (stronger HTTPS patterns or gRPC over **publicly trusted TLS** / **ACME** — **no private-CA device identity**)

**Architecture decisions (identity & TLS):** device auth stays **Bearer / JWT-style tokens** (no mTLS client certs). **HTTPS** for control plane and public APIs uses **publicly trusted certificates (ACME recommended)**; no org-wide private CA / cert issuance / revocation product track for devices.

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
   - Optional: edit `scripts/alerts/suggestions.yml` to customize per-alert triage hints shown by the mock receiver (`steps` and optional `commands` templates).
2. Submit a manual alert:
   - `./scripts/alerts/send-test-alert.sh`
   - `./scripts/alerts/send-test-alert.sh --alertname MasqueConnectIPTunLinkUpFailures --severity warning` (exercise alert-specific runbook + suggestions)
   - `./scripts/alerts/send-test-alert.sh --dry-run --alertname MasqueConnectIPTunOpenEchoFallback` (preview JSON payload without POST)
3. Confirm delivery in receiver output logs.
   - The mock receiver prints an Alertmanager summary block (status, grouped alerts, `runbook_url`) and per-alert suggested next steps for common CONNECT-IP/TUN alerts.

Current Alertmanager webhook target is:
- `http://host.docker.internal:5001/alerts`

## Deploy and rollback scripts

- Deploy: `./scripts/deploy/deploy.sh staging` (optional Go build into `releases/<ts>/bin/`, same ldflags as CI; `BUILD_GO=0` to skip)
- Rollback: `./scripts/deploy/rollback.sh`
- Dataplane preflight (enabled by default): deploy/rollback run `scripts/deploy/dataplane-preflight.sh` to check `ip`, `sysctl`, `nft`/`iptables`, `/dev/net/tun`, and current `masque-server` environment. Disable with `DEPLOY_DATAPLANE_PREFLIGHT=0` or `ROLLBACK_DATAPLANE_PREFLIGHT=0`.
- Preflight logs are archived to `logs/deploy/preflight-<timestamp>.log` (deploy) and `logs/deploy/rollback-preflight-<timestamp>.log` (rollback). Override paths with `DEPLOY_PREFLIGHT_LOG_DIR` / `ROLLBACK_PREFLIGHT_LOG_DIR`.

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
- RBAC management UI at `/admin/rbac` (`admin.rbac.write`): create roles, bind permissions, sync user roles; template roles `auditor` / `ops` / `security` ship with migrations (see `开发需求.md` §3.3)
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
- Phase 2b kernel-forward checks (capabilities + metrics + rules):
  - `RUN_PHASE2B_KERNEL=1 ./scripts/staging/full-check.sh`
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
- `RUN_PHASE2B_KERNEL=1` validate CONNECT-IP kernel-forward/shared/NAT capabilities + metrics + alerts
- `RUN_AUTHZ_HMAC_CHECK=1` run control-plane `/api/v1/server/authorize` HMAC gate check (requires `AUTHZ_HMAC_SECRET` for signed path; set `AUTHZ_HMAC_REQUIRED_EXPECTED=1` when staging enforces required mode)
- `RUN_MULTI_NODE_HA_CHECK=1` run multi-node HA gate (`MASQUE_NODE_URLS=http://10.0.0.11:8443,http://10.0.0.12:8443`, `EXPECTED_HEALTHY_NODES=2`) and verify Prometheus has at least N healthy `job="masque-server"` targets; optional `MASQUE_LB_URL=http://masque-lb:8443` enables LB health/capability consistency check
- `STRICT_HA_NO_UNEXPECTED=1` (with HA check enabled) fails if Prometheus reports **extra** healthy `masque-server` instances not listed in `MASQUE_NODE_URLS` (stricter drift detection for staging/prod evidence)
- `RUN_IPV4_SCOPE_CHECK=1` runs `scripts/staging/ipv4-dataplane-scope-check.sh` to assert the IPv4-only product boundary string remains in `capabilities` `rfc9484.not_implemented`
- When HA check runs, `full-check` report includes a **Multi-node HA Capability Matrix** section (node/LB endpoint vs key capability flags) for ops/audit evidence.
- HA check also includes **Prometheus Target Detail (masque-server)** (instance/health/scrape_url/last_error) to quickly identify failing node targets.
- HA check enforces node-list matching: normalized `MASQUE_NODE_URLS` host:port entries must appear as healthy Prometheus `instance` targets (prevents false pass by unrelated healthy nodes).
- HA report now includes **HA Instance Matching** section with `expected_missing_in_healthy` and `healthy_not_in_expected` rows for quick diff diagnostics.
- `CONTROL_PLANE_URL`, `MASQUE_SERVER_URL`, `PROMETHEUS_URL`, `ALERTMANAGER_URL`
- `phase2b-kernel-check.sh` also enforces current product boundary in capabilities: `CONNECT-IP TCP or IPv6 datagram relay` remains listed as `not_implemented` for this phase.

GitHub Actions staging gate:
- Open `CI` workflow via `workflow_dispatch`
- Set `run_phase2b_kernel=true`, then provide staging URLs (`control_plane_url`, `masque_server_url`, `prometheus_url`, `alertmanager_url`, optional `loki_url`/`grafana_url`)
- The job `phase2b-kernel-staging` runs `scripts/staging/full-check.sh` with `RUN_PHASE2B_KERNEL=1`
- Optional auth hardening gate: set `run_authz_hmac_check=true`, provide secret `STAGING_AUTHZ_HMAC_SECRET`, and set `authz_hmac_required_expected=1` after staging enables `MASQUE_AUTHORIZE_HMAC_REQUIRED=true`
- Optional HA gate: set `run_multi_node_ha_check=true`, provide `masque_node_urls` (comma-separated node base URLs), `expected_healthy_nodes` (e.g. `2`), and optional `masque_lb_url` to verify LB capability profile matches backend node baseline
- Optional stricter HA: set `strict_ha_no_unexpected=true` when you want `STRICT_HA_NO_UNEXPECTED=1` passed into the HA script (recommended once `MASQUE_NODE_URLS` is authoritative)
- Optional IPv4 scope gate: set `run_ipv4_scope_check=true` to run the capabilities-only IPv4 boundary check (`RUN_IPV4_SCOPE_CHECK=1`)

### VPN NAT fault-injection (Actions + local)

- **GitHub Actions:** open workflow **`VPN NAT fault-injection script`** (`vpn-nat-fault-injection-dispatch.yml`) via **Actions → VPN NAT fault-injection script → Run workflow**.
  - Every run executes **`smoke`**: `bash -n` on `scripts/vpn-nat-backend-fault-injection.sh` plus **`--dry-run`** / **`--dry-run --restore-only`** (no SSH).
  - Optional: enable **`run_remote_fault_injection`** to run the full script after smoke passes (SSH as **`root`**, same behavior as local).
- **Local / bastion (recommended for production hosts):** `MASQUE_HOST=<masque_ip> ./scripts/vpn-nat-backend-fault-injection.sh` (see `--help` for `--dry-run`, `--skip-*`, `--restore-only`).
- **Secrets for the optional remote job** (repository or organization):
  - **`MASQUE_FAULT_INJECTION_HOST`** — SSH target (hostname or IP) for `root@${MASQUE_FAULT_INJECTION_HOST}`.
  - **`FAULT_SSH_PRIVATE_KEY`** — private key with access to that host (OpenSSH PEM / RSA / ed25519; `webfactory/ssh-agent` loads it).
  - **`MASQUE_FAULT_INJECTION_CLIENT_HOST`** (optional) — forwarded to **`CLIENT_HOST`** for `vpn-tunnel-bench.sh`; defaults to **`103.6.4.5`** when unset or empty.
- **Networking:** GitHub-hosted runners must reach the SSH port on **`MASQUE_FAULT_INJECTION_HOST`**. If the masque host is only on a private network, use a **self-hosted runner** with outbound SSH access and set **`runs-on`** for job **`remote-fault-injection`** in `vpn-nat-fault-injection-dispatch.yml` (for example `runs-on: self-hosted`).
