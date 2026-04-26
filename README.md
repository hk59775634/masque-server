# MASQUE VPN Monorepo (Phase 2 in progress)

This repository contains a closed-loop implementation and M2 upgrades:

- `control-plane/`: Laravel API (`/api/v1/...`) for provisioning, token auth, and policy management
- `masque-server/`: Go service with control-plane authorization callback and ACL enforcement
- `linux-client/`: Go CLI with `activate` / `connect` / `status` / `disconnect` / `doctor` / `version` / `config …` and route/DNS apply+restore
- `docs/adr/`: initial architecture decisions

## Step-by-step development baseline

1. Start control-plane:
   - `cd control-plane`
   - `php artisan migrate`
   - `php artisan serve --host=127.0.0.1 --port=8000`
2. Start masque-server:
   - `cd ../masque-server`
   - `go mod tidy`
   - `go run ./cmd/server version` (optional; prints build metadata, overridable with `-ldflags -X main.version=...`)
   - `CONTROL_PLANE_URL=http://127.0.0.1:8000 go run ./cmd/server`
3. Use API to create user + activation code + policy:
   - `POST /api/v1/users`
   - `POST /api/v1/devices/activation-code`
   - `POST /api/v1/users/{id}/policy` or `POST /api/v1/devices/{id}/policy`
4. Activate and connect from CLI:
   - `cd ../linux-client`
   - `go mod tidy`
   - `go run ./cmd/client version` (optional; same `-ldflags -X main.version=...` pattern as server)
   - `go run ./cmd/client activate -control-plane http://127.0.0.1:8000 -fingerprint fp-demo-001 -code XXXX-YYYY` (optional `-verify`: control plane before activate; masque `/healthz` after — masque failure only warns, config still saved)
   - `go run ./cmd/client doctor -h` (optional probes: control plane + masque `/healthz`, `-strict` requires masque URL)
   - `go run ./cmd/client config show` (token redacted) or `config path` / `config export` / `config import -i file [-force] [-verify]`
   - `go run ./cmd/client status -live` (local summary + `GET /api/v1/devices/self`); `status -json` / `status -json -live` for machine-readable output
   - `sudo go run ./cmd/client connect` (optional `connect -check` to require control plane `GET /api/v1/devices/self` OK before masque)
   - `sudo go run ./cmd/client disconnect`

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
- server metrics endpoint for Prometheus (`/metrics`); `/healthz` JSON includes `version` string (link-time `main.version`)
- Go binaries: `client version` / `server version` subcommands for release traceability (`-ldflags -X main.version|commit|date`)
- CI workflow for Laravel + Go components (`.github/workflows/ci.yml`); Go job builds each binary with `-ldflags` (`main.version` = `ci-<run>` or tag name, `main.commit` = short SHA, `main.date` UTC) and runs `version` smoke
- observability stack via Docker Compose (`ops/observability/docker-compose.yml`)
- basic deploy and rollback scripts (`scripts/deploy/deploy.sh`, `scripts/deploy/rollback.sh`)
- Prometheus alert rules + Alertmanager baseline config

Not yet implemented:

- Real MASQUE data plane tunnel handling
- mTLS certificate issuance/rotation/revocation
- Fine-grained RBAC beyond the `is_admin` flag (roles/permissions matrix)

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
