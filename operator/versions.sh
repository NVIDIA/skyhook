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
	local key="${1:-}"
	local yq
	yq="$(_versions_yq)" || return 1

	case "${key}" in
	envtestK8s)
		"${yq}" -r '.envtest.kubernetes' "${_versions_file}"
		;;
	kindVersion | kindNodeImage)
		"${yq}" -r '.kind.nodeImage' "${_versions_file}"
		;;
	kindBinary)
		"${yq}" -r '.kind.binary' "${_versions_file}"
		;;
	ciKindNodeImagesJson)
		"${yq}" -o=json -I=0 '.ci.kindNodeImages' "${_versions_file}"
		;;
	ciPrimaryKindNodeImage)
		"${yq}" -r '.ci.primaryKindNodeImage' "${_versions_file}"
		;;
	*)
		printf 'unknown version key: %s\n' "${key}" >&2
		return 1
		;;
	esac
}

_versions_export() {
	local envtest_k8s
	local kind_version
	local kind_binary
	local ci_kind_node_images_json
	local ci_primary_kind_node_image

	envtest_k8s="$(_versions_get envtestK8s)" || return 1
	kind_version="$(_versions_get kindVersion)" || return 1
	kind_binary="$(_versions_get kindBinary)" || return 1
	ci_kind_node_images_json="$(_versions_get ciKindNodeImagesJson)" || return 1
	ci_primary_kind_node_image="$(_versions_get ciPrimaryKindNodeImage)" || return 1

	export ENVTEST_K8S_VERSION="${ENVTEST_K8S_VERSION:-${envtest_k8s}}"
	export KIND_VERSION="${KIND_VERSION:-${kind_version}}"
	export KIND_NODE_IMAGE_VERSION="${KIND_NODE_IMAGE_VERSION:-${KIND_VERSION}}"
	export KIND_BINARY_VERSION="${KIND_BINARY_VERSION:-${kind_binary}}"
	export CI_KIND_NODE_IMAGE_VERSIONS_JSON="${CI_KIND_NODE_IMAGE_VERSIONS_JSON:-${ci_kind_node_images_json}}"
	export CI_PRIMARY_KIND_NODE_IMAGE_VERSION="${CI_PRIMARY_KIND_NODE_IMAGE_VERSION:-${ci_primary_kind_node_image}}"
}

_versions_print() {
	printf 'envtest-k8s-version=%s\n' "$(_versions_get envtestK8s)"
	printf 'kind-version=%s\n' "$(_versions_get kindVersion)"
	printf 'kind-node-image-version=%s\n' "$(_versions_get kindNodeImage)"
	printf 'kind-binary-version=%s\n' "$(_versions_get kindBinary)"
	printf 'ci-kind-node-image-versions-json=%s\n' "$(_versions_get ciKindNodeImagesJson)"
	printf 'ci-primary-kind-node-image-version=%s\n' "$(_versions_get ciPrimaryKindNodeImage)"
}

_versions_main() {
	set -euo pipefail

	case "${1:-}" in
	--get)
		if [ "$#" -ne 2 ]; then
			printf 'usage: %s --get <key>\n' "${_versions_script}" >&2
			return 1
		fi
		_versions_get "$2"
		;;
	--print)
		_versions_print
		;;
	--export)
		_versions_export
		export -p ENVTEST_K8S_VERSION KIND_VERSION KIND_NODE_IMAGE_VERSION KIND_BINARY_VERSION CI_KIND_NODE_IMAGE_VERSIONS_JSON CI_PRIMARY_KIND_NODE_IMAGE_VERSION
		;;
	"" | -h | --help)
		cat <<EOF
usage: ${_versions_script} --get <key>
       ${_versions_script} --print
       source ${_versions_script}

keys:
  envtestK8s
  kindVersion
  kindNodeImage
  kindBinary
  ciKindNodeImagesJson
  ciPrimaryKindNodeImage
EOF
		;;
	*)
		printf 'unknown argument: %s\n' "$1" >&2
		return 1
		;;
	esac
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
	_versions_main "$@"
else
	_versions_export
fi
