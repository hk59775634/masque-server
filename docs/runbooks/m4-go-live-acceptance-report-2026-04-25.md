# M4 Go-Live Acceptance Report (2026-04-25)

## 1) Scope

- Project: MASQUE VPN (control-plane + masque-server + linux-client)
- Stage: M4 production readiness close-out
- Report time (UTC): 2026-04-25T15:50:00+00:00

## 2) Executed Checks

- `php artisan audit:verify-chain`
  - Result: PASS
  - Evidence: `Verification passed. processed=1252`
- `php artisan test`
  - Result: PASS
  - Evidence: `2 passed (2 assertions)`
- `php artisan route:list --path=admin`
  - Result: PASS
  - Evidence: 8 admin routes loaded, including:
    - `admin.operation-token`
    - `admin.users.force-logout`
    - `admin.users.force-logout-scope`
- `./scripts/staging/smoke-test.sh`
  - Result: PASS
  - Evidence: `[smoke] all checks passed.`
- `./scripts/staging/full-check.sh`
  - Result: PASS
  - Evidence:
    - `[PASS] smoke-test.sh`
    - `[PASS] prometheus target masque-server is up`
    - `[PASS] prometheus alert rules loaded`
    - `[PASS] manual test alert submitted`
    - `[PASS] alertmanager received manual test alert`
  - Report artifact: `scripts/staging/reports/full-check-20260425-154901.md`

## 3) Security and Governance Status

- One-time confirmation token enabled for high-risk admin operations: DONE
- Admin idle session timeout middleware enabled: DONE
- User session revocation and force logout (single + batch scope): DONE
- Audit log tamper-evident hash chain:
  - schema fields `prev_hash` + `entry_hash`: DONE
  - write-time chain sealing: DONE
  - backfill + verify commands: DONE

## 4) Residual Risks and Notes

- `full-check` in this run skipped k6 load (`RUN_K6=0`); performance capacity sign-off should use a dedicated load window.
- Continue periodic execution of:
  - `php artisan audit:verify-chain`
  - `scripts/staging/full-check.sh`

## 5) Acceptance Decision

- Current decision: **Pass**
- Reason:
  - Application-level checks passed.
  - Infrastructure/runtime checks passed after `masque-server` startup.
  - Audit-integrity verification passed.

## 6) Sign-off Template

- Operator:
- Date:
- Environment:
- Decision (Pass / Conditional Pass / Fail):
- Notes:
