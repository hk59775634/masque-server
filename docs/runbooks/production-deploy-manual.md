# Production Deploy Manual

## 1. Pre-check

- Verify environment:
  - `APP_ENV=production`
  - `APP_DEBUG=false`
  - `ALLOW_FIRST_USER_ADMIN=false`
- Confirm latest backup exists (DB + critical config).
- Confirm health status:
  - `https://www.afbuyers.com/login`
  - `http://127.0.0.1:8443/healthz`
  - Prometheus targets all `UP`.

## 2. Deploy

1. Pull latest code to release host.
2. Run deploy script:
   - `./scripts/deploy/deploy.sh prod`
   - Optional restarts (same semantics as rollback): `DEPLOY_SYSTEMCTL_UNITS="nginx php8.2-fpm masque-server" ./scripts/deploy/deploy.sh prod` — uses `scripts/deploy/systemctl-restart-lib.sh` (`systemctl restart` per unit; root or `sudo -n`).
   - Go: by default the script runs `go build` for `masque-server` and `linux-client` into `releases/<ts>/bin/` with `-ldflags` (`main.version` / `main.commit` / `main.date`). Values are resolved in `scripts/deploy/go-build-flags.sh` (env `DEPLOY_VERSION` or `VERSION`, GitHub tag/CI vars when present, else `git describe`, else `deploy-<timestamp>`). Set `BUILD_GO=0` to skip if the host has no Go toolchain. Example override: `DEPLOY_VERSION=v1.2.3 ./scripts/deploy/deploy.sh prod`.
3. Run post-deploy checks:
   - `php artisan test` (or smoke-test in staging-like environment)
   - `./scripts/staging/smoke-test.sh`

## 3. Validate observability

- Prometheus:
  - check `masque_connect_requests_total` increments.
- Grafana:
  - dashboard `MASQUE Server Overview` has live data.
- Alertmanager:
  - run `./scripts/alerts/send-test-alert.sh`
  - verify webhook receiver/notifier got alert.

## 4. Rollback

If severe regression happens:

1. Execute rollback:
   - `./scripts/deploy/rollback.sh`
   - Optional restarts: `ROLLBACK_SYSTEMCTL_UNITS="nginx php8.2-fpm masque-server" ./scripts/deploy/rollback.sh` (same `systemctl restart` helper as deploy: `scripts/deploy/systemctl-restart-lib.sh`; root or `sudo -n`).
2. Restart service stack (nginx/php-fpm/masque-server). If releases were built with Go, binaries live under `current/bin/` after `rollback.sh` switches the symlink—point unit files or manual restarts at `masque-server` there when applicable.
3. Re-run smoke checks.
4. Record incident timeline and root cause.

## 5. High-risk admin operations

- Admin role changes and device disable/ban require one-time confirmation token (5 minutes, single-use).
- Force logout user session is available in Admin user panel and also requires one-time confirmation token.
- Batch force logout by scope is available (all users / non-admin users, excluding current operator), also requires one-time confirmation token.
- Always verify audit log after high-risk operations.

## 6. Audit integrity

- After deploying hash-chain migration, run:
  - `php artisan audit:backfill-chain`
- Verify chain integrity:
  - `php artisan audit:verify-chain`
- If verification fails, freeze admin writes and investigate immediately.

## 7. Go-live acceptance report

- After all checks, produce and archive a dated acceptance report.
- Example reference:
  - `docs/runbooks/m4-go-live-acceptance-report-2026-04-25.md`
