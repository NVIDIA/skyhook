#!/usr/bin/env bash
# SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
# SPDX-License-Identifier: Apache-2.0
#
# Interactive release-tag helper. Prompts for a component and a bump
# (major/minor/patch), optionally a release candidate, computes the next version,
# creates the git tag, and (after a separate explicit confirmation) pushes it.
#
# Pushing a <component>/v* tag triggers the CI release workflow, so the push is
# always a distinct, opt-in step. RC tags use the dotted `-rc.N` form so version
# sort orders pre-releases correctly (see docs/release-process.md).
#
# The tag is created on the current HEAD. Make sure the release commit (chart
# bump, CHANGELOG cut, etc.) is already committed and checked out before tagging.
# Both the tag creation and the push are gated behind explicit y/N confirms.

set -euo pipefail

cd "$(dirname "$0")/.."

if [[ ! -t 0 ]]; then
    echo "ERROR: release-tag.sh is interactive; run it from a terminal." >&2
    exit 1
fi

# Bump a vX.Y.Z version. Echoes the bumped vX.Y.Z.
bump_version() {
    local ver="${1#v}" kind="$2" major minor patch
    IFS=. read -r major minor patch <<<"$ver"
    case "$kind" in
        major) printf 'v%d.0.0\n' "$((major + 1))" ;;
        minor) printf 'v%d.%d.0\n' "$major" "$((minor + 1))" ;;
        patch) printf 'v%d.%d.%d\n' "$major" "$minor" "$((patch + 1))" ;;
        *)
            echo "ERROR: bump_version: unknown kind '$kind'" >&2
            return 1
            ;;
    esac
}

# Next RC number for a base version: one greater than the highest existing
# <component>/<base>-rc.<n>, or 1 if none exist.
next_rc() {
    local component="$1" base="$2" highest
    highest=$(git tag -l "${component}/${base}-rc.*" |
        sed -E "s#^${component}/${base}-rc\.##" | sort -n | tail -1)
    printf '%d\n' "$(((${highest:-0}) + 1))"
}

PS3="Select component: "
select COMPONENT in operator agent chart cli; do
    [[ -n "$COMPONENT" ]] && break
    echo "invalid selection" >&2
done

LATEST_FINAL=$(git tag -l "${COMPONENT}/v*" --sort=-v:refname |
    grep -E "^${COMPONENT}/v[0-9]+\.[0-9]+\.[0-9]+$" | head -1)
LATEST_FINAL="${LATEST_FINAL#"${COMPONENT}/"}"
if [[ -z "$LATEST_FINAL" ]]; then
    echo "ERROR: no final ${COMPONENT}/vX.Y.Z tags found" >&2
    exit 1
fi
echo "Latest final tag: ${COMPONENT}/${LATEST_FINAL}"

PS3="Select bump: "
select BUMP in major minor patch; do
    [[ -n "$BUMP" ]] && break
    echo "invalid selection" >&2
done
BASE=$(bump_version "$LATEST_FINAL" "$BUMP")

read -r -p "Release candidate? [y/N] " is_rc
if [[ "$is_rc" =~ ^[yY]$ ]]; then
    RC=$(next_rc "$COMPONENT" "$BASE")
    VERSION="${BASE}-rc.${RC}"
else
    VERSION="$BASE"
fi
TAG="${COMPONENT}/${VERSION}"

if git rev-parse -q --verify "refs/tags/${TAG}" >/dev/null; then
    echo "ERROR: tag ${TAG} already exists" >&2
    exit 1
fi

HEAD_REF="$(git rev-parse --abbrev-ref HEAD) @ $(git rev-parse --short HEAD)"

echo
echo "About to tag (on ${HEAD_REF}): ${TAG}"
echo "Reminder: the release commit (chart bump, CHANGELOG) should already be committed at HEAD."
read -r -p "Create local tag ${TAG}? [y/N] " ok
if [[ ! "$ok" =~ ^[yY]$ ]]; then
    echo "aborted"
    exit 0
fi
git tag "$TAG"
echo "Created local tag ${TAG}."

echo
echo "Pushing ${TAG} will TRIGGER THE CI RELEASE for this component."
read -r -p "Push ${TAG} to origin now? [y/N] " push_ok
if [[ "$push_ok" =~ ^[yY]$ ]]; then
    git push origin "$TAG"
    echo "Pushed ${TAG}."
else
    echo "Not pushed. Push later with: git push origin ${TAG}"
fi
