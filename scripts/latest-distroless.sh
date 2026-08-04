#!/usr/bin/env bash
# SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
# SPDX-License-Identifier: Apache-2.0
#
# Resolve the newest NVIDIA distroless base image that actually exists in the
# container registry, and pin it to a digest.
#
# https://developer.download.nvidia.com/distroless-oss/versions.json advertises a
# release days before the matching image lands in nvcr.io, so a build that trusts
# it fails with a 404 on the base image for as long as the two are out of step.
# This asks the registry instead - the tag list cannot be ahead of the images it
# lists.
#
# The distroless repositories are public, so oras reads them anonymously and no
# NGC credentials are involved.

set -euo pipefail

ORAS="${ORAS:-oras}"

_die() {
	printf 'latest-distroless.sh: %s\n' "$*" >&2
	exit 1
}

# Anchoring the match is what discards everything that is not a release tag: the
# cosign artifacts the registry stores as tags (sha256-....sig/.sbom/.vex) and the
# parallel -dev stream both fail it. sort -V is a version sort, so 4.0.9 orders
# before 4.0.10 where a lexical sort would not.
_latest_version() {
	local prefix_re="$1" major="$2"
	# No match is a normal outcome the caller reports, but grep exits 1 for it and
	# pipefail would abort before that message is ever reached.
	{ grep -E "^${prefix_re}v${major}\.[0-9]+\.[0-9]+$" || true; } |
		sed -e "s/^${prefix_re}v//" |
		sort -V |
		tail -n 1
}

_usage() {
	cat <<EOF
Resolve the newest distroless base image present in the registry.

usage: scripts/latest-distroless.sh --repo <ref> --major <N> [options]

--repo         registry repository, e.g. nvcr.io/nvidia/distroless/static
--major        major version to stay within, e.g. 4. Required: a major bump of
               the base image is a deliberate change, not something a build
               should pick up on its own.
--tag-prefix   text before the version in the tag, e.g. '3.13-' for
               nvcr.io/nvidia/distroless/python tags like 3.13-v4.0.8.
               Default empty.
--platforms    comma-separated platforms the tag must provide.
               Default linux/amd64,linux/arm64.
--print        emit lower-case hyphenated 'key=value' lines for GitHub Actions
               step outputs (>> \$GITHUB_OUTPUT) instead of the bare version.

Prints the bare version (no leading v) by default:
  scripts/latest-distroless.sh --repo nvcr.io/nvidia/distroless/static --major 4
  scripts/latest-distroless.sh --repo nvcr.io/nvidia/distroless/python --major 4 --tag-prefix 3.13-

With --print:
  distroless-version=4.0.0
  distroless-tag=v4.0.0
  distroless-digest=sha256:...
EOF
}

_main() {
	local repo="" major="" prefix="" platforms="linux/amd64,linux/arm64" print=""

	while [ $# -gt 0 ]; do
		case "$1" in
		--repo)
			repo="${2:-}"
			shift 2
			;;
		--major)
			major="${2:-}"
			shift 2
			;;
		--tag-prefix)
			prefix="${2:-}"
			shift 2
			;;
		--platforms)
			platforms="${2:-}"
			shift 2
			;;
		--print)
			print=1
			shift
			;;
		-h | --help)
			_usage
			return 0
			;;
		*)
			_die "unknown argument: $1"
			;;
		esac
	done

	[ -n "${repo}" ] || _die "--repo is required (see --help)"
	[ -n "${major}" ] || _die "--major is required (see --help)"
	command -v "${ORAS}" >/dev/null 2>&1 || _die "oras is required: https://oras.land/docs/installation"
	command -v jq >/dev/null 2>&1 || _die "jq is required"

	local tags version tag digest manifest platform missing=""

	tags="$("${ORAS}" repo tags "${repo}")" || _die "could not list tags for ${repo}"
	# A prefix like '3.13-' carries dots that would otherwise match any character.
	version="$(printf '%s\n' "${tags}" | _latest_version "${prefix//./\\.}" "${major}")"
	[ -n "${version}" ] || _die "no v${major}.x release tag found for ${repo} with prefix '${prefix}'"

	tag="${prefix}v${version}"

	digest="$("${ORAS}" manifest fetch --descriptor "${repo}:${tag}" | jq -r '.digest // empty')" ||
		_die "could not resolve a digest for ${repo}:${tag}"
	[ -n "${digest}" ] || _die "${repo}:${tag} returned no digest"

	# A tag whose push only half-finished resolves fine above and then fails on one
	# architecture's build runner much later, so check the index covers them now.
	manifest="$("${ORAS}" manifest fetch "${repo}:${tag}")" ||
		_die "could not read the manifest for ${repo}:${tag}"
	for platform in ${platforms//,/ }; do
		printf '%s' "${manifest}" | jq -e --arg p "${platform}" \
			'[.manifests[]? | select(.platform.os + "/" + .platform.architecture == $p)] | length > 0' \
			>/dev/null 2>&1 || missing="${missing} ${platform}"
	done
	[ -z "${missing}" ] || _die "${repo}:${tag} is missing platform(s):${missing}"

	if [ -n "${print}" ]; then
		printf 'distroless-version=%s\n' "${version}"
		printf 'distroless-tag=%s\n' "${tag}"
		printf 'distroless-digest=%s\n' "${digest}"
	else
		printf '%s\n' "${version}"
	fi
}

_main "$@"
