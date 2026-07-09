#!/usr/bin/env bash
# Copyright (c) NVIDIA CORPORATION.  All rights reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

_versions_script="${BASH_SOURCE[0]}"
_versions_dir="$(cd "$(dirname "${_versions_script}")" && pwd)"
_versions_file="${VERSIONS_FILE:-${_versions_dir}/versions.yaml}"

_versions_yq() {
	if [ -n "${YQ:-}" ] && [ -x "${YQ}" ]; then
		printf '%s\n' "${YQ}"
		return 0
	fi

	if [ -x "${_versions_dir}/bin/yq" ]; then
		printf '%s\n' "${_versions_dir}/bin/yq"
		return 0
	fi

	if command -v yq >/dev/null 2>&1; then
		command -v yq
		return 0
	fi

	printf 'versions.sh requires yq. Run `make -C operator yq` or install yq on PATH.\n' >&2
	return 1
}

_versions_get() {
	local path="${1:-}"
	local yq
	yq="$(_versions_yq)" || return 1

	if [ -z "${path}" ]; then
		printf 'usage: %s --get <yq-path> [yq-args...]\n' "${_versions_script}" >&2
		return 1
	fi

	# The path is a bare yq key expression (e.g. kind.binary); prepend the root
	# dot and forward any trailing yq flags (e.g. -o=json for array output).
	shift
	local out
	out="$("${yq}" -r "$@" ".${path}" "${_versions_file}")" || return 1
	# yq yields "null" for a missing path and still exits 0; treat that as an
	# error so a typo'd key fails loudly instead of returning an empty value.
	if [ "${out}" = "null" ]; then
		printf 'versions.sh: no value at .%s in %s\n' "${path}" "${_versions_file##*/}" >&2
		return 1
	fi
	printf '%s\n' "${out}"
}

# Leaf keys read straight from versions.yaml: every scalar or sequence node,
# skipping map containers and sequence elements (a sequence is one value). All
# names below derive from these paths, so adding a key to versions.yaml surfaces
# it in --print/--export with no edit here.
_versions_leaves() {
	local yq
	yq="$(_versions_yq)" || return 1
	"${yq}" -r '.. | select(tag != "!!map") | select((path | .[-1] | tag) != "!!int") | path | join(".")' "${_versions_file}"
}

# Names derive straight from the yq leaf path, so the versions.yaml key
# structure is the single source of naming (e.g. envtest.k8s.version yields
# ENVTEST_K8S_VERSION / envtest-k8s-version). env name: upper-case with '.'/'-'
# collapsed to '_'; out key: lower-case with '.'/'_' collapsed to '-'.
_versions_env_name() { printf '%s' "$1" | tr '[:lower:]' '[:upper:]' | tr '.-' '__'; }
_versions_out_key() { printf '%s' "$1" | tr '[:upper:]' '[:lower:]' | tr '._' '--'; }

# Scalars print raw; sequences print as compact JSON so array values survive.
_versions_value() {
	local path="$1" yq
	yq="$(_versions_yq)" || return 1
	if [ "$("${yq}" -r ".${path} | tag" "${_versions_file}")" = "!!seq" ]; then
		"${yq}" -o=json -I=0 ".${path}" "${_versions_file}"
	else
		"${yq}" -r ".${path}" "${_versions_file}"
	fi
}

# Print `export NAME=value` statements for every leaf so the output can be
# eval'd or sourced: `eval "$(versions.sh --export)"`. Names are UPPER_SNAKE
# (shell convention); values are %q shell-quoted so any characters (quotes,
# the JSON array) survive the eval intact.
_versions_export() {
	local path env value
	while IFS= read -r path; do
		[ -n "${path}" ] || continue
		env="$(_versions_env_name "${path}")"
		value="$(_versions_value "${path}")" || return 1
		printf 'export %s=%q\n' "${env}" "${value}"
	done < <(_versions_leaves)
}

_versions_print() {
	local path value
	while IFS= read -r path; do
		[ -n "${path}" ] || continue
		value="$(_versions_value "${path}")" || return 1
		printf '%s=%s\n' "$(_versions_out_key "${path}")" "${value}"
	done < <(_versions_leaves)
}

_versions_main() {
	set -euo pipefail

	case "${1:-}" in
	--get)
		shift
		_versions_get "$@"
		;;
	--print)
		_versions_print
		;;
	--export)
		_versions_export
		;;
	"" | -h | --help)
		cat <<EOF
Read tool/runtime versions from ${_versions_file##*/}.

usage: ${_versions_script} --get <yq-path> [yq-args...]
       ${_versions_script} --print
       ${_versions_script} --export
       source ${_versions_script}

--get     print one value at <yq-path> (the leading . is added for you) and
          forward any trailing yq flags. A missing path is an error, e.g.:
            ${_versions_script} --get envtest.k8s.version
            ${_versions_script} --get kind.binary
            ${_versions_script} --get ci.kindNodeImages -o=json -I=0
--print   enumerate every key and emit lower-case hyphenated 'key=value' lines
          for GitHub Actions step outputs (>> \$GITHUB_OUTPUT).
--export  enumerate every key and emit UPPER_SNAKE 'export NAME=value' lines
          for the shell: eval "\$(${_versions_script##*/} --export)".
source    (bash) applies the --export assignments to your shell; in other
          shells use: eval "\$(${_versions_script##*/} --export)".

--print / --export names derive from each key's path: dots become the
separators, so kind.binary -> kind-binary / KIND_BINARY and envtest.k8s.version ->
envtest-k8s-version / ENVTEST_K8S_VERSION. Add a key to the file and it appears
in both with no change here.
EOF
		;;
	*)
		printf 'unknown argument: %s\n' "$1" >&2
		return 1
		;;
	esac
}

# Run directly (bash via the shebang): dispatch CLI args. Sourced into bash:
# apply the exports to the caller's shell. Sourced into another shell (zsh,
# etc.) the bash-isms above misbehave, so point the user at the portable form
# instead of failing with a misleading yq error.
if [ -z "${BASH_VERSION:-}" ]; then
	printf 'versions.sh: source needs bash; in other shells run: eval "$(./%s --export)"\n' "${0##*/}" >&2
elif [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
	_versions_main "$@"
else
	# Capture first so a mid-stream _versions_export failure does not leave the
	# caller's shell with a partial set of exports.
	if ! __versions_exports="$(_versions_export)"; then
		printf 'versions.sh: failed to read %s\n' "${_versions_file##*/}" >&2
		return 1
	fi
	eval "${__versions_exports}"
	unset __versions_exports
fi
