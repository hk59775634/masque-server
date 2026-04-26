#!/usr/bin/env bash
# Shared Go link flags for masque-server + linux-client (deploy + CI).
# Usage: SOURCE_ROOT=/path/to/repo [DEPLOY_VERSION=v1] [DEPLOY_TIMESTAMP=...] . path/to/go-build-flags.sh
# Then: go build -trimpath -ldflags "${GO_LDFLAGS}" ...

if [[ -z "${BASH_VERSION:-}" ]]; then
	echo "go-build-flags.sh: bash required" >&2
	return 1 2>/dev/null || exit 1
fi

_this_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SOURCE_ROOT="${SOURCE_ROOT:-$(cd "${_this_dir}/../.." && pwd)}"

VERSION="${DEPLOY_VERSION:-${VERSION:-}}"
if [[ -z "${VERSION}" ]] && [[ "${GITHUB_REF_TYPE:-}" == "tag" ]] && [[ -n "${GITHUB_REF_NAME:-}" ]]; then
	VERSION="${GITHUB_REF_NAME}"
fi
if [[ -z "${VERSION}" ]] && [[ -n "${GITHUB_RUN_NUMBER:-}" ]]; then
	VERSION="ci-${GITHUB_RUN_NUMBER}"
fi
if [[ -z "${VERSION}" ]] && command -v git >/dev/null 2>&1 && git -C "${SOURCE_ROOT}" rev-parse --git-dir >/dev/null 2>&1; then
	VERSION="$(git -C "${SOURCE_ROOT}" describe --tags --always --dirty 2>/dev/null || true)"
fi
if [[ -z "${VERSION}" ]] && [[ -n "${DEPLOY_TIMESTAMP:-}" ]]; then
	VERSION="deploy-${DEPLOY_TIMESTAMP}"
fi
[[ -n "${VERSION}" ]] || VERSION="unknown"

COMMIT=""
if [[ -n "${GITHUB_SHA:-}" ]] && [[ "${#GITHUB_SHA}" -ge 7 ]]; then
	COMMIT="${GITHUB_SHA:0:7}"
elif command -v git >/dev/null 2>&1 && git -C "${SOURCE_ROOT}" rev-parse --git-dir >/dev/null 2>&1; then
	COMMIT="$(git -C "${SOURCE_ROOT}" rev-parse --short HEAD 2>/dev/null || true)"
fi
[[ -n "${COMMIT}" ]] || COMMIT="unknown"

DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

export GO_LDFLAGS="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${DATE}"
export GO_BUILD_VERSION="${VERSION}"
export GO_BUILD_COMMIT="${COMMIT}"
export GO_BUILD_DATE="${DATE}"
