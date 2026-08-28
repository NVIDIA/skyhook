#!/usr/bin/env bash

# SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
# SPDX-License-Identifier: Apache-2.0
#
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
# http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Regenerates the architecture diagram PNGs one directory up. See README.md.
#
# The SVGs are intermediates written to a temp dir, not committed: diagrams.py
# is the source of truth and the PNGs are the artifact the docs reference, so
# committing a third representation would only create a way for them to drift.

set -euo pipefail

cd "$(dirname "$0")"

OUT_DIR="$(cd .. && pwd)"
SVG_DIR="$(mktemp -d)"
trap 'rm -rf "$SVG_DIR"' EXIT

# Pinned so a resvg release cannot silently change how the committed PNGs render.
RESVG="${RESVG:-@resvg/resvg-js-cli@2.6.2-beta.1}"

# 2x for legibility on high-DPI displays; the SVGs are authored at 1x.
ZOOM="${ZOOM:-2}"

command -v npx >/dev/null 2>&1 || {
  echo "error: npx not found. Node.js is required to run ${RESVG}." >&2
  exit 1
}

# Importing theme.py would otherwise leave a __pycache__ directory in the docs tree.
PYTHONDONTWRITEBYTECODE=1 python3 diagrams.py --out "$SVG_DIR"

for svg in "$SVG_DIR"/*.svg; do
  name="$(basename "$svg" .svg)"
  npx --yes "$RESVG" --fit-zoom "$ZOOM" "$svg" "${OUT_DIR}/${name}.png" >/dev/null
  echo "rendered ${name}.png"
done

echo
echo "Rendered to ${OUT_DIR}. Open the PNGs before committing —"
echo "overlapping connectors and clipped labels are invisible in the source."
