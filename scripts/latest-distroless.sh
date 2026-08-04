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
# nvcr.io serves the standard OCI Registry v2 API and hands out anonymous pull
# tokens, so resolving needs no NGC credentials.

set -euo pipefail

_registry="${DISTROLESS_REGISTRY:-nvcr.io}"
_scratch=""

_cleanup() { [ -n "${_scratch}" ] && rm -rf "${_scratch}"; }
trap _cleanup EXIT

_die() {
	printf 'latest-distroless.sh: %s\n' "$*" >&2
	exit 1
}

_pull_token() {
	local repo="$1" token
	token="$(curl -fsSL --retry 3 --retry-delay 2 --max-time 30 \
		"https://${_registry}/proxy_auth?scope=repository:${repo}:pull" |
		jq -r '.token // empty')" ||
		_die "could not reach ${_registry} to authenticate for ${repo}"
	[ -n "${token}" ] || _die "no anonymous pull token issued for ${repo}"
	printf '%s\n' "${token}"
}

# The registry caps a tag page at 1000 entries and distroless/python carries
# ~1800 tags, so a single request silently truncates the list and hides the
# newest tags. Follow the RFC 5988 Link header until the registry stops offering
# a next page.
_all_tags() {
	local repo="$1" token="$2"
	local next="/v2/${repo}/tags/list?n=1000"
	local headers="${_scratch}/tags.headers"

	while [ -n "${next}" ]; do
		curl -fsSL --retry 3 --retry-delay 2 --max-time 60 -D "${headers}" \
			-H "Authorization: Bearer ${token}" "https://${_registry}${next}" |
			jq -r '.tags[]? // empty' ||
			_die "could not list tags for ${repo}"
		next="$(sed -n 's/^[Ll]ink:[[:space:]]*<\([^>]*\)>.*/\1/p' "${headers}" | tr -d '\r')"
	done
}

# Anchoring the match is what discards everything that is not a release tag: the
# cosign artifacts the registry stores as tags (sha256-….sig/.sbom/.vex) and the
# parallel -dev stream both fail it. Sorting is numeric per component because tag
# order from the registry is arbitrary and lexical order puts 4.0.9 above 4.0.10.
_latest_version() {
	local prefix_re="$1" major="$2"
	# No match is a normal outcome the caller reports, but grep exits 1 for it and
	# pipefail would abort before that message is ever reached.
	{ grep -E "^${prefix_re}v${major}\.[0-9]+\.[0-9]+$" || true; } |
		sed -e "s/^${prefix_re}v//" |
		sort -t. -k1,1n -k2,2n -k3,3n |
		tail -n 1
}

# Resolve tag -> digest, and confirm the tag is an index covering every platform
# the build targets. A tag whose push only half-finished resolves fine here and
# then fails on one architecture's runner much later, so check it up front.
_resolve_digest() {
	local repo="$1" tag="$2" token="$3" platforms="$4"
	local headers="${_scratch}/manifest.headers"
	local body="${_scratch}/manifest.json"
	local digest platform missing=""

	curl -fsSL --retry 3 --retry-delay 2 --max-time 30 -D "${headers}" -o "${body}" \
		-H "Authorization: Bearer ${token}" \
		-H "Accept: application/vnd.oci.image.index.v1+json,application/vnd.docker.distribution.manifest.list.v2+json" \
		"https://${_registry}/v2/${repo}/manifests/${tag}" ||
		_die "could not read the manifest for ${repo}:${tag}"

	digest="$(sed -n 's/^[Dd]ocker-[Cc]ontent-[Dd]igest:[[:space:]]*\(.*\)/\1/p' "${headers}" | tr -d '\r')"
	[ -n "${digest}" ] || _die "${_registry} returned no digest for ${repo}:${tag}"

	for platform in ${platforms//,/ }; do
		jq -e --arg p "${platform}" \
			'[.manifests[]? | select(.platform.os + "/" + .platform.architecture == $p)] | length > 0' \
			"${body}" >/dev/null 2>&1 || missing="${missing} ${platform}"
	done
	[ -z "${missing}" ] || _die "${repo}:${tag} is missing platform(s):${missing}"

	printf '%s\n' "${digest}"
}

_usage() {
	cat <<EOF
Resolve the newest distroless base image present in ${_registry}.

usage: scripts/latest-distroless.sh --repo <repo> --major <N> [options]

--repo         registry repository, e.g. nvidia/distroless/static
--major        major version to stay within, e.g. 4. Required: a major bump of
               the base image is a deliberate change, not something a build
               should pick up on its own.
--tag-prefix   text before the version in the tag, e.g. '3.13-' for
               nvidia/distroless/python tags like 3.13-v4.0.8. Default empty.
--platforms    comma-separated platforms the tag must provide.
               Default linux/amd64,linux/arm64.
--print        emit lower-case hyphenated 'key=value' lines for GitHub Actions
               step outputs (>> \$GITHUB_OUTPUT) instead of the bare version.

Prints the bare version (no leading v) by default:
  scripts/latest-distroless.sh --repo nvidia/distroless/static --major 4
  scripts/latest-distroless.sh --repo nvidia/distroless/python --major 4 --tag-prefix 3.13-

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
	command -v jq >/dev/null 2>&1 || _die "jq is required"

	# Explicit template: BSD mktemp ignores TMPDIR unless one is given, so a bare
	# -d writes to /var/folders on macOS and diverges from the Linux CI runners.
	_scratch="$(mktemp -d "${TMPDIR:-/tmp}/latest-distroless.XXXXXX")"

	local token version tag digest
	token="$(_pull_token "${repo}")"
	# A prefix like '3.13-' carries dots that would otherwise match any character.
	version="$(_all_tags "${repo}" "${token}" | _latest_version "${prefix//./\\.}" "${major}")"
	[ -n "${version}" ] || _die "no v${major}.x release tag found for ${repo} with prefix '${prefix}'"

	tag="${prefix}v${version}"
	digest="$(_resolve_digest "${repo}" "${tag}" "${token}" "${platforms}")"

	if [ -n "${print}" ]; then
		printf 'distroless-version=%s\n' "${version}"
		printf 'distroless-tag=%s\n' "${tag}"
		printf 'distroless-digest=%s\n' "${digest}"
	else
		printf '%s\n' "${version}"
	fi
}

_main "$@"
