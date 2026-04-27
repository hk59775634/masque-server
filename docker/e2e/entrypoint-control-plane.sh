#!/usr/bin/env bash
set -euo pipefail
cd /app

rm -f .env
cp .env.example .env

composer install --no-interaction --prefer-dist --no-dev

php artisan key:generate --force --no-interaction

for key in APP_URL MASQUE_SERVER_URL DB_CONNECTION DB_DATABASE CACHE_STORE SESSION_DRIVER QUEUE_CONNECTION; do
	if grep -q "^${key}=" .env 2>/dev/null; then
		grep -v "^${key}=" .env >.env.tmp && mv .env.tmp .env
	fi
done

{
	echo "APP_URL=${APP_URL:-http://control-plane:8000}"
	echo "MASQUE_SERVER_URL=${MASQUE_SERVER_URL:-http://masque:8443}"
	echo "DB_CONNECTION=sqlite"
	echo "DB_DATABASE=${DB_DATABASE:-/app/database/database.sqlite}"
	echo "CACHE_STORE=array"
	echo "SESSION_DRIVER=array"
	echo "QUEUE_CONNECTION=sync"
} >>.env

mkdir -p "$(dirname "${DB_DATABASE:-/app/database/database.sqlite}")"
rm -f "${DB_DATABASE:-/app/database/database.sqlite}"
touch "${DB_DATABASE:-/app/database/database.sqlite}"
chmod 666 "${DB_DATABASE:-/app/database/database.sqlite}" 2>/dev/null || true

php artisan migrate --force --no-interaction

exec php artisan serve --host=0.0.0.0 --port=8000 --no-interaction
