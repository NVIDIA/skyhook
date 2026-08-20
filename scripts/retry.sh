#!/usr/bin/env bash
# SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
# SPDX-License-Identifier: Apache-2.0
#
# Re-run a command until it succeeds, with exponential backoff between attempts.
#
# CI reaches out to Docker Hub, ghcr.io, nvcr.io and GitHub releases from steps
# that have no retry of their own, so a single reset connection fails the whole
# job with an error that reads like a real problem ("image is not published")
# rather than the blip it was. Wrap those fetches in this so only a sustained
# outage stops the run.
#
# Reserve it for reads that are safe to repeat. A push, a tag move, or anything
# else that mutates a registry does not belong here.

set -euo pipefail

_die() {
	printf 'retry.sh: %s\n' "$*" >&2
	exit 1
}

_usage() {
	cat <<EOF
Re-run a command until it succeeds, backing off between attempts.

usage: scripts/retry.sh [options] -- <command> [args...]

--attempts     total tries, including the first. Default 3.
--delay        seconds to wait after the first failure. Doubles each attempt.
               Default 5.
--max-delay    ceiling for the backoff. Default 30.
--label        name used in the progress lines. Default: the command.

Exits with the command's own status once attempts run out, so a caller that
distinguishes exit codes still sees the real one.

  scripts/retry.sh -- docker manifest inspect kindest/node:v1.33.12
  scripts/retry.sh --attempts 5 --label 'oras tags' -- oras repo tags "\$REPO"
EOF
}

_main() {
	local attempts=3 delay=5 max_delay=30 label=""

	while [ $# -gt 0 ]; do
		case "$1" in
		--attempts)
			attempts="${2:-}"
			shift 2
			;;
		--delay)
			delay="${2:-}"
			shift 2
			;;
		--max-delay)
			max_delay="${2:-}"
			shift 2
			;;
		--label)
			label="${2:-}"
			shift 2
			;;
		-h | --help)
			_usage
			return 0
			;;
		--)
			shift
			break
			;;
		*)
			_die "unknown argument: $1 (the command goes after --)"
			;;
		esac
	done

	[ $# -gt 0 ] || _die "no command given (see --help)"
	[ -n "${label}" ] || label="$*"

	local re='^[0-9]+$'
	if ! [[ "${attempts}" =~ ${re} ]] || [ "${attempts}" -lt 1 ]; then
		_die "--attempts must be a positive integer"
	fi
	[[ "${delay}" =~ ${re} ]] || _die "--delay must be a non-negative integer"
	[[ "${max_delay}" =~ ${re} ]] || _die "--max-delay must be a non-negative integer"

	# Buffer stdout per attempt and only replay the winning one. Callers capture
	# this in $(...) or pipe it to jq, and a failed attempt that already wrote a
	# partial response would otherwise be concatenated with the retry that
	# succeeded, producing malformed output instead of an error. stderr streams
	# through untouched so progress and failures stay visible live.
	local out
	out="$(mktemp)" || _die "could not create a temporary file"
	# shellcheck disable=SC2064 # expand out now; it is fixed for the run.
	trap "rm -f '${out}'" EXIT

	local attempt=1 status=0 wait="${delay}"
	while :; do
		status=0
		"$@" >"${out}" || status=$?
		if [ "${status}" -eq 0 ]; then
			cat "${out}"
			return 0
		fi
		[ "${attempt}" -ge "${attempts}" ] && break

		printf 'retry.sh: %s failed (exit %d), attempt %d/%d, retrying in %ds\n' \
			"${label}" "${status}" "${attempt}" "${attempts}" "${wait}" >&2
		sleep "${wait}"
		attempt=$((attempt + 1))
		wait=$((wait * 2))
		[ "${wait}" -gt "${max_delay}" ] && wait="${max_delay}"
	done

	cat "${out}"
	printf 'retry.sh: %s failed (exit %d) after %d attempts\n' \
		"${label}" "${status}" "${attempts}" >&2
	return "${status}"
}

_main "$@"
