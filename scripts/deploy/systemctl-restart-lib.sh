#!/usr/bin/env bash
# shellcheck shell=bash
# Shared helpers for deploy.sh / rollback.sh — source this file; do not execute.

masque_systemctl_invoke() {
	if [[ "${EUID:-0}" -eq 0 ]]; then
		systemctl "$@"
	else
		sudo -n systemctl "$@"
	fi
}

# Args: log tag, space-separated unit names (may be unset/empty).
# Returns 1 if units string is empty → caller should print placeholder hints.
# Exits 1 if units non-empty but systemctl missing.
# If units are only whitespace: warn and return 0.
masque_systemctl_restart_list() {
	local tag="${1:?}"
	local units="${2:-}"

	if [[ -z "${units}" ]]; then
		return 1
	fi
	if ! command -v systemctl >/dev/null 2>&1; then
		echo "[${tag}] systemd units requested but systemctl not in PATH" >&2
		exit 1
	fi

	read -r -a _masque_units <<< "${units}"
	local has=0
	for u in "${_masque_units[@]}"; do
		[[ -n "${u}" ]] || continue
		has=1
		echo "[${tag}] systemctl restart ${u}"
		masque_systemctl_invoke restart "${u}"
	done
	if [[ "${has}" -eq 0 ]]; then
		echo "[${tag}] unit list was empty after parsing; skipping systemctl" >&2
	fi
	return 0
}
