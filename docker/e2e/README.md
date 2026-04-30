# Docker E2E: linux-client vs control-plane + masque-server

Runs the same flow as `scripts/dev/e2e-client-connect-dry-run.sh` inside Compose: create API user → activation code → `client activate` → `client connect -dry-run` (no host routes/DNS).

## Prerequisites

- Docker with **Compose v2** (`docker compose`) is recommended. **Compose v1** (`docker-compose` 1.29.x) may hit `KeyError: 'ContainerConfig'` when recreating images on newer Docker engines; upgrade the Compose plugin or use `docker compose` from Docker CLI v2.

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
| `vpn-probe`   | Optional profile **`vpn-internet`**: root + TUN + `connect-ip-tun` + curl + speedtest (see section below) |

Override URLs if you change the compose project (defaults use Docker DNS names `control-plane` and `masque`).

## VPN internet probe (Linux host recommended)

End-to-end **data plane**: `POST /api/v1/devices/bootstrap` → `connect-ip-tun` (TUN + QUIC CONNECT-IP + split routes) → `curl` **ipinfo.io** / **google.com** → optional **`speedtest-cli`**.

Masque is configured with **`QUIC_LISTEN_ADDR`** and **CONNECT-IP TUN + managed NAT** (see `docker-compose.yml`). The probe container runs **privileged** with **`/dev/net/tun`** so the client can create `tun0` and install routes.

```bash
docker compose -f docker/e2e/docker-compose.yml --profile vpn-internet up --build --abort-on-container-exit --remove-orphans vpn-probe
```

Compose v1 equivalent (if supported):

```bash
docker-compose -f docker/e2e/docker-compose.yml --profile vpn-internet up --build --abort-on-container-exit --remove-orphans vpn-probe
```

- Set **`SKIP_SPEEDTEST=1`** in the environment for the `vpn-probe` service if outbound speedtest is blocked.
- **`speedtest-cli`** may fail behind strict firewalls; curl checks are the primary pass criteria.
- Requires outbound internet from the Docker bridge (typical on Linux; **Docker Desktop** networking may differ).

## Notes

- Compose defaults use **HTTP** from control-plane to masque (`MASQUE_SERVER_URL=http://masque:8443`). If you run masque with **`LISTEN_TLS_CERT` / `LISTEN_TLS_KEY`**, set **`MASQUE_SERVER_URL=https://masque:8443`** and ensure the PHP image trusts the server chain (**public CA / ACME**, or a **dev-only** cert injected for CI — **not** an org-wide private CA product requirement); the `e2e` container must reach masque over the same scheme.
- Control-plane entrypoint runs `composer install --no-dev` on each container start (cold start ~1–2 minutes on first pull).
- Host `database/*.sqlite` files are excluded from the build context (`.dockerignore`); the container always starts with an empty SQLite file and full migrations.
