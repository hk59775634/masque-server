# Docker E2E: linux-client vs control-plane + masque-server

Runs the same flow as `scripts/dev/e2e-client-connect-dry-run.sh` inside Compose: create API user → activation code → `client activate` → `client connect -dry-run` (no host routes/DNS).

## Prerequisites

- Docker with **Compose** (v2: `docker compose`, or v1: `docker-compose`).

## Run (from repository root)

```bash
docker compose -f docker/e2e/docker-compose.yml up --build --abort-on-container-exit --remove-orphans
```

With legacy Compose v1:

```bash
docker-compose -f docker/e2e/docker-compose.yml up --build --abort-on-container-exit --remove-orphans
```

Teardown (optional):

```bash
docker compose -f docker/e2e/docker-compose.yml down -v
```

## Services

| Service         | Role |
|----------------|------|
| `control-plane` | PHP 8.4, fresh SQLite per start, `php artisan serve` on `:8000` |
| `masque`        | Go `masque-server`, `CONTROL_PLANE_URL=http://control-plane:8000` |
| `e2e`           | Debian + `masque-client` binary + curl/jq; exits 0 on success |

Override URLs if you change the compose project (defaults use Docker DNS names `control-plane` and `masque`).

## Notes

- Compose defaults use **HTTP** from control-plane to masque (`MASQUE_SERVER_URL=http://masque:8443`). If you run masque with **`LISTEN_TLS_CERT` / `LISTEN_TLS_KEY`**, set **`MASQUE_SERVER_URL=https://masque:8443`** (and mount a CA the PHP image trusts, or use dev certs consistently); the `e2e` container must reach masque over the same scheme.
- Control-plane entrypoint runs `composer install --no-dev` on each container start (cold start ~1–2 minutes on first pull).
- Host `database/*.sqlite` files are excluded from the build context (`.dockerignore`); the container always starts with an empty SQLite file and full migrations.
