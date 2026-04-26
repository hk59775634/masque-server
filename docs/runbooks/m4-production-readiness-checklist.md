# M4 Production Readiness Checklist

## 1) Security Baseline

- [ ] `APP_ENV=production` in runtime environment.
- [ ] `APP_DEBUG=false` in production.
- [ ] `ALLOW_FIRST_USER_ADMIN=false` in production.
- [ ] Rotate admin credentials and enforce strong password policy.
- [ ] HTTPS termination enabled and valid TLS cert installed.
- [ ] Verify security headers in responses:
  - `X-Frame-Options: DENY`
  - `X-Content-Type-Options: nosniff`
  - `Referrer-Policy: no-referrer`
  - `Content-Security-Policy` present
  - `Strict-Transport-Security` on HTTPS

## 2) Access Control

- [ ] Confirm only designated operators have `is_admin=true`.
- [ ] Remove temporary/test admin accounts.
- [ ] Verify admin-only pages return `403` for non-admin.
- [ ] Set `ADMIN_SESSION_IDLE_TIMEOUT_MINUTES` and verify idle admin sessions auto-expire.

## 3) API Hardening

- [ ] Verify write endpoints enforce throttling limits.
- [ ] Confirm auth failures and policy updates are recorded in `audit_logs`.
- [ ] Confirm JWT/token expiration policy and renewal flow are documented.

## 4) Observability & Alerts

- [ ] Prometheus targets all `UP` (`/api/v1/targets`).
- [ ] Alert rules loaded (`/api/v1/rules`).
- [ ] Alertmanager reachable and receiving test alert.
- [ ] Grafana dashboard panels show real data for connect traffic.

## 5) Deployment & Recovery

- [ ] Validate `scripts/deploy/deploy.sh` in staging.
- [ ] Validate `scripts/deploy/rollback.sh` in staging.
- [ ] Keep at least 2 releases for rollback safety.

## 6) Validation Run

- [ ] Run `scripts/staging/full-check.sh`.
- [ ] Optionally run load scenario: `RUN_K6=1 scripts/staging/full-check.sh`.
- [ ] Archive generated report under `scripts/staging/reports/`.
- [ ] Run `php artisan audit:backfill-chain` once after deploy/migration.
- [ ] Run `php artisan audit:verify-chain` and keep output as evidence.
- [ ] Run `php artisan audit:archive-old --days=180` and verify archive policy is active.
