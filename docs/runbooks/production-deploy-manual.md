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

### TLS / ACME baseline (required)

- Public endpoints (control-plane Web/API and public masque HTTPS entry) must use **publicly trusted certificates**.
- Recommended approach: **ACME** automation (e.g. certbot / acme.sh / cloud LB managed certs).
- Private CA for endpoint identity is **out of scope** for this project line; do not rely on self-signed chains in production.
- Keep renewal SLO explicit:
  - auto-renew job enabled
  - renewal dry-run tested
  - alert if cert expires within 21 days
- Verify server names and SANs match:
  - control-plane domain(s): e.g. `www.afbuyers.com`
  - masque public domain(s): e.g. `masque.afbuyers.com`

## 2. Deploy

1. Pull latest code to release host.
2. Run deploy script:
   - `./scripts/deploy/deploy.sh prod`
   - Optional restarts (same semantics as rollback): `DEPLOY_SYSTEMCTL_UNITS="nginx php8.2-fpm masque-server" ./scripts/deploy/deploy.sh prod` — uses `scripts/deploy/systemctl-restart-lib.sh` (`systemctl restart` per unit; root or `sudo -n`).
   - Go: by default the script runs `go build` for `masque-server` and `linux-client` into `releases/<ts>/bin/` with `-ldflags` (`main.version` / `main.commit` / `main.date`). Values are resolved in `scripts/deploy/go-build-flags.sh` (env `DEPLOY_VERSION` or `VERSION`, GitHub tag/CI vars when present, else `git describe`, else `deploy-<timestamp>`). Set `BUILD_GO=0` to skip if the host has no Go toolchain. Example override: `DEPLOY_VERSION=v1.2.3 ./scripts/deploy/deploy.sh prod`.
3. Run post-deploy checks:
   - `php artisan test` (or smoke-test in staging-like environment)
   - `./scripts/staging/smoke-test.sh`
4. Control-plane database migration (when the release ships RBAC extension migrations such as `2026_04_30_120000_add_rbac_management_permission_and_template_roles`):
   - From the `control-plane` app directory: `php artisan migrate --force`
   - Follow **RBAC migration checklist** below before closing the change.

#### RBAC migration checklist

- After `migrate`: confirm `roles` includes `admin`, `auditor`, `ops`, `security`; confirm `permissions` includes `admin.rbac.write` and `permission_role` attaches it (and other permissions) to the `admin` role.
- Confirm at least one break-glass operator can manage RBAC: `is_admin=true` and/or `admin` role (with 2FA challenge if enabled).
- If the database already contained custom rows for names `auditor`, `ops`, or `security`, review the migration’s `updateOrInsert` / permission sync behaviour before migrating in production.
- **Rollback:** avoid blind `php artisan migrate:rollback` in production; prefer forward-fix. Rolling back the last batch removes template roles and `admin.rbac.write` and can strand operators without RBAC UI access—plan session invalidation and role reassignment if you must roll back.

## 3. Validate observability

- Prometheus:
  - check `masque_connect_requests_total` increments.
- Grafana:
  - dashboard `MASQUE Server Overview` has live data.
- Alertmanager:
  - run `./scripts/alerts/send-test-alert.sh`
  - verify webhook receiver/notifier got alert.

### TLS checks after deploy

- Verify certificate issuer/trust:
  - `echo | openssl s_client -connect www.afbuyers.com:443 -servername www.afbuyers.com 2>/dev/null | openssl x509 -noout -issuer -subject -dates`
- Verify masque public endpoint chain (replace hostname):
  - `echo | openssl s_client -connect masque.afbuyers.com:443 -servername masque.afbuyers.com 2>/dev/null | openssl x509 -noout -issuer -subject -dates`
- Ensure expiry (`notAfter`) meets policy and matches ACME expected renewal window.

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
- RBAC 管理页（`/admin/rbac`）：修改 **admin** 角色的权限绑定，或向任意角色**新授予**敏感权限（`admin.access`、`admin.policy.write`、`admin.session.revoke`、`admin.rbac.write`）时，需要一次性确认码；用户管理员特权变化与策略页规则一致。

### Authorize HMAC（控制面 ↔ masque）

- 先在 **masque-server** 与 **control-plane** 配置相同的 `MASQUE_AUTHORIZE_HMAC_SECRET`（及控制面侧等价变量，见 `.env.example` 注释）。
- 双方冒烟通过后再将 `MASQUE_AUTHORIZE_HMAC_REQUIRED=true`（staging 建议在验证 `scripts/staging/authz-hmac-check.sh` 后默认开启）。
- 回滚：若 `/api/v1/server/authorize` 回调大量失败，先在**两侧**将 `MASQUE_AUTHORIZE_HMAC_REQUIRED=false`，保留密钥以便快速再次开启。

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
