#!/usr/bin/env bash
set -euo pipefail

ENVIRONMENT="${1:-staging}"
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
RELEASES_DIR="${ROOT_DIR}/releases"
CURRENT_LINK="${ROOT_DIR}/current"
TIMESTAMP="$(date +%Y%m%d%H%M%S)"
NEW_RELEASE="${RELEASES_DIR}/${TIMESTAMP}"

echo "[deploy] environment=${ENVIRONMENT}"
mkdir -p "${RELEASES_DIR}"

echo "[deploy] creating release: ${NEW_RELEASE}"
mkdir -p "${NEW_RELEASE}"

echo "[deploy] syncing source files..."
rsync -a --exclude '.git' --exclude 'releases' --exclude 'current' "${ROOT_DIR}/" "${NEW_RELEASE}/"

# Optional Go binaries (ldflags from scripts/deploy/go-build-flags.sh, same as CI).
# Set BUILD_GO=0 to skip, or DEPLOY_VERSION=1.2.3 to override auto-detected version.
BUILD_GO="${BUILD_GO:-1}"
if [[ "${BUILD_GO}" == "1" ]] && command -v go >/dev/null 2>&1; then
	# shellcheck source=go-build-flags.sh
	SOURCE_ROOT="${ROOT_DIR}" DEPLOY_TIMESTAMP="${TIMESTAMP}" . "${ROOT_DIR}/scripts/deploy/go-build-flags.sh"
	BIN_DIR="${NEW_RELEASE}/bin"
	mkdir -p "${BIN_DIR}"
	echo "[deploy] building Go binaries (version=${GO_BUILD_VERSION} commit=${GO_BUILD_COMMIT}) -> ${BIN_DIR}"
	( cd "${NEW_RELEASE}/masque-server" && go mod tidy && go build -trimpath -ldflags "${GO_LDFLAGS}" -o "${BIN_DIR}/masque-server" ./cmd/server )
	( cd "${NEW_RELEASE}/linux-client" && go mod tidy && go build -trimpath -ldflags "${GO_LDFLAGS}" -o "${BIN_DIR}/masque-client" ./cmd/client )
	chmod +x "${BIN_DIR}/masque-server" "${BIN_DIR}/masque-client"
	echo "[deploy] masque-server version:"
	"${BIN_DIR}/masque-server" version || true
	echo "[deploy] masque-client version:"
	"${BIN_DIR}/masque-client" version || true
elif [[ "${BUILD_GO}" == "1" ]]; then
	echo "[deploy] BUILD_GO=1 but go not in PATH; skipped Go compile (install Go or set BUILD_GO=0)"
fi

PREV_RELEASE=""
if [[ -L "${CURRENT_LINK}" ]]; then
	PREV_RELEASE="$(readlink -f "${CURRENT_LINK}")"
fi

echo "[deploy] switching current symlink..."
ln -sfn "${NEW_RELEASE}" "${CURRENT_LINK}"

# Do not replace production SQLite with the (often empty) file from the git checkout.
if [[ -n "${PREV_RELEASE}" && -f "${PREV_RELEASE}/control-plane/database/database.sqlite" ]]; then
	echo "[deploy] preserving control-plane SQLite from previous release"
	cp -a "${PREV_RELEASE}/control-plane/database/database.sqlite" "${NEW_RELEASE}/control-plane/database/database.sqlite"
fi

# Laravel (PHP-FPM www-data) must write SQLite, cache, sessions, logs.
if [[ -d "${CURRENT_LINK}/control-plane" ]]; then
	echo "[deploy] fixing control-plane writable dirs for www-data..."
	chown -R www-data:www-data "${CURRENT_LINK}/control-plane/storage" "${CURRENT_LINK}/control-plane/bootstrap/cache" "${CURRENT_LINK}/control-plane/database" 2>/dev/null || true
	chmod 775 "${CURRENT_LINK}/control-plane/database" 2>/dev/null || true
fi

# shellcheck source=systemctl-restart-lib.sh
. "${ROOT_DIR}/scripts/deploy/systemctl-restart-lib.sh"
if masque_systemctl_restart_list "deploy" "${DEPLOY_SYSTEMCTL_UNITS:-}"; then
	:
else
	echo "[deploy] restart services placeholder (customize for your server)"
	echo "  - php-fpm reload"
	echo "  - nginx reload"
	echo "  - masque-server restart (binary: \${ROOT_DIR}/current/bin/masque-server if built)"
	echo "  - optional: DEPLOY_SYSTEMCTL_UNITS=\"nginx php8.2-fpm masque-server\" $0 ${ENVIRONMENT}"
fi

echo "[deploy] done. current -> ${NEW_RELEASE}"
