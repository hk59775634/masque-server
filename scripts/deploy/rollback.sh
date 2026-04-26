#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
RELEASES_DIR="${ROOT_DIR}/releases"
CURRENT_LINK="${ROOT_DIR}/current"

if [[ ! -d "${RELEASES_DIR}" ]]; then
  echo "[rollback] releases directory not found: ${RELEASES_DIR}"
  exit 1
fi

mapfile -t RELEASES < <(ls -1 "${RELEASES_DIR}" | sort -r)
if (( ${#RELEASES[@]} < 2 )); then
  echo "[rollback] need at least two releases to rollback"
  exit 1
fi

TARGET="${RELEASES_DIR}/${RELEASES[1]}"
echo "[rollback] switching current -> ${TARGET}"
ln -sfn "${TARGET}" "${CURRENT_LINK}"

# Optional: ROLLBACK_SYSTEMCTL_UNITS="nginx php8.2-fpm masque-server" ./scripts/deploy/rollback.sh
# shellcheck source=systemctl-restart-lib.sh
. "${ROOT_DIR}/scripts/deploy/systemctl-restart-lib.sh"
if masque_systemctl_restart_list "rollback" "${ROLLBACK_SYSTEMCTL_UNITS:-}"; then
	:
else
	echo "[rollback] restart services placeholder (customize for your server)"
	echo "  - php-fpm reload"
	echo "  - nginx reload"
	echo "  - masque-server restart (if you use deploy.sh Go build: binaries live under \${ROOT_DIR}/current/bin/)"
	echo "  - optional: ROLLBACK_SYSTEMCTL_UNITS=\"nginx php8.2-fpm masque-server\" $0"
fi

echo "[rollback] done"
