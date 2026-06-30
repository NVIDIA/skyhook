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

# SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
# SPDX-License-Identifier: Apache-2.0
#
# Generate a per-component CHANGELOG.md from git history.
#
# Why this exists instead of a single `git-cliff --include-path ...` call:
# git-cliff path-filters the commit set FIRST, then looks for release tags only
# among the surviving commits. In this monorepo most release tags sit on commits
# that touch no files under the component's own path (chart bumps, CI fixes,
# cross-component features, ride-along agent releases) and some live only on
# release branches. git-cliff therefore silently drops those release sections and
# rolls their commits into [Unreleased]. See orhun/git-cliff#1122 and #208.
#
# Fix: the authoritative list of releases lives in `git tag`, not in the filtered
# commit walk. We take the release boundaries from `git tag` and ask git-cliff to
# render each section over an explicit `prevTag..curTag` range. `git log A..B`
# resolves across branches, so release-branch tags work as endpoints, and a tag
# whose commit was path-filtered is still a valid boundary. --include-path keeps
# doing its real job: choosing which commits appear *inside* each section.
#
# Usage:
#   scripts/gen-changelog.sh <operator|agent|chart|cli> [next-version]
#
# Run with no args on a TTY for an interactive component/action picker.
#
# If [next-version] (e.g. v0.2.0) is given, commits since the latest tag are
# rendered as that version instead of [Unreleased] -- use this when cutting a
# release. Otherwise unreleased commits go under [Unreleased].

set -euo pipefail

cd "$(dirname "$0")/.."

# Colorize interactive output only when stdout is a terminal and NO_COLOR is
# unset; piped/CI runs get empty strings so committed logs stay escape-free.
if [[ -t 1 && -z "${NO_COLOR:-}" ]]; then
    C_RESET=$'\033[0m' C_BOLD=$'\033[1m' C_DIM=$'\033[2m'
    C_CYAN=$'\033[36m' C_GREEN=$'\033[32m' C_YELLOW=$'\033[33m' C_RED=$'\033[31m'
else
    C_RESET='' C_BOLD='' C_DIM='' C_CYAN='' C_GREEN='' C_YELLOW='' C_RED=''
fi

COMPONENT=""
NEXT_VERSION=""

# Bump a vX.Y.Z tag. Echoes the bumped vX.Y.Z.
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

if [[ $# -eq 0 && -t 0 ]]; then
    # Interactive release helper: pick a component + action, then fall through to
    # the same generation logic the non-interactive path uses.
    # Force one menu item per line: `select` otherwise packs items into columns
    # by $COLUMNS, and the embedded ANSI codes throw off its width math.
    COLUMNS=1
    PS3="${C_BOLD}${C_CYAN}Select component: ${C_RESET}"
    select COMPONENT in operator agent chart cli; do
        [[ -n "$COMPONENT" ]] && break
        echo "${C_RED}invalid selection${C_RESET}" >&2
    done

    LATEST_FINAL=$(git tag -l "${COMPONENT}/v*" --sort=-v:refname |
        grep -E "^${COMPONENT}/v[0-9]+\.[0-9]+\.[0-9]+$" | head -1)
    LATEST_FINAL="${LATEST_FINAL#"${COMPONENT}/"}"
    if [[ -z "$LATEST_FINAL" ]]; then
        echo "${C_RED}ERROR: no final ${COMPONENT}/vX.Y.Z tags found${C_RESET}" >&2
        exit 1
    fi
    echo "${C_DIM}Latest final tag:${C_RESET} ${C_BOLD}${COMPONENT}/${LATEST_FINAL}${C_RESET}"

    MAJOR_NEXT=$(bump_version "$LATEST_FINAL" major)
    MINOR_NEXT=$(bump_version "$LATEST_FINAL" minor)
    PATCH_NEXT=$(bump_version "$LATEST_FINAL" patch)

    # Menu text is colorized, so dispatch on the numeric $REPLY rather than the
    # captured (escape-laden) $ACTION string.
    PS3="${C_BOLD}${C_CYAN}Select action: ${C_RESET}"
    select ACTION in \
        "Major (${LATEST_FINAL} ${C_DIM}->${C_RESET} ${C_GREEN}${MAJOR_NEXT}${C_RESET})" \
        "Minor (${LATEST_FINAL} ${C_DIM}->${C_RESET} ${C_GREEN}${MINOR_NEXT}${C_RESET})" \
        "Patch (${LATEST_FINAL} ${C_DIM}->${C_RESET} ${C_GREEN}${PATCH_NEXT}${C_RESET})" \
        "Regenerate only ${C_DIM}(Unreleased)${C_RESET}"; do
        [[ -n "$ACTION" ]] && break
        echo "${C_RED}invalid selection${C_RESET}" >&2
    done

    case "$REPLY" in
        1) NEXT_VERSION="$MAJOR_NEXT" ;;
        2) NEXT_VERSION="$MINOR_NEXT" ;;
        3) NEXT_VERSION="$PATCH_NEXT" ;;
        4) NEXT_VERSION="" ;;
    esac

    if [[ -n "$NEXT_VERSION" ]]; then
        read -r -p "${C_YELLOW}Cut ${C_BOLD}${COMPONENT}/${NEXT_VERSION}${C_RESET}${C_YELLOW} and write ${COMPONENT}'s CHANGELOG.md?${C_RESET} [y/N] " reply
        if [[ ! "$reply" =~ ^[yY]$ ]]; then
            echo "${C_YELLOW}aborted${C_RESET}"
            exit 0
        fi
    fi
elif [[ $# -eq 0 ]]; then
    echo "ERROR: no args and not a TTY. Usage: $0 <operator|agent|chart|cli> [next-version]" >&2
    exit 1
else
    COMPONENT="$1"
    NEXT_VERSION="${2:-}"
fi

case "$COMPONENT" in
    cli)
        INCLUDE_PATH="operator/cmd/cli"
        OUTPUT="operator/cmd/cli/CHANGELOG.md"
        ;;
    operator | agent | chart)
        INCLUDE_PATH="$COMPONENT"
        OUTPUT="$COMPONENT/CHANGELOG.md"
        ;;
    *)
        echo "ERROR: unknown component '$COMPONENT' (want operator|agent|chart|cli)" >&2
        exit 1
        ;;
esac

# Only final releases are section boundaries. Pre-releases (-rc, -test, ...) must
# not become their own sections or split a range, so we match X.Y.Z exactly.
FINAL_RE="^${COMPONENT}/v[0-9]+\.[0-9]+\.[0-9]+$"
# Restrict git-cliff's own tag detection to the same final tags, so intermediate
# rc tags inside a prevTag..curTag range don't create extra split points.
TAG_PATTERN="${COMPONENT}/v[0-9]+\\.[0-9]+\\.[0-9]+$"

# Final tags, oldest -> newest.
# readarray/mapfile require bash 4+; use a while-read loop for bash 3 compat (macOS default shell).
TAGS=()
while IFS= read -r _tag; do TAGS+=("$_tag"); done < <(git tag -l "${COMPONENT}/v*" --sort=v:refname | grep -E "$FINAL_RE" || true)

if [[ ${#TAGS[@]} -eq 0 ]]; then
    echo "ERROR: no final ${COMPONENT}/vX.Y.Z tags found" >&2
    exit 1
fi

# Common git-cliff flags. --offline disables the GitHub API enrichment, which
# otherwise panics on an unauthenticated 403 and adds `by [@user]` links we don't
# keep in committed changelogs.
CLIFF_COMMON=(--offline --include-path "${INCLUDE_PATH}/**" --tag-pattern "$TAG_PATTERN" --strip all)

# Render one released section. git-cliff can't see a tag whose commit was
# path-filtered out, so we pass --tag explicitly to scope+label the range; that
# stamps today's date, so we rewrite the heading with the tag's real date.
released_section() {
    local cur="$1" range="$2" body date
    # $range is intentionally unquoted: it is a single "prev..cur" token with no
    # spaces, and quoting it would pass an empty string as an extra argument when
    # the variable is empty (which never happens here, but the pattern is fragile).
    # shellcheck disable=SC2086
    body=$(git-cliff "${CLIFF_COMMON[@]}" --tag "$cur" $range 2>/dev/null) || true
    if [[ -z "${body//[[:space:]]/}" ]]; then
        echo "WARNING: released_section: empty output for ${cur} (range: ${range})" >&2
        return 0
    fi
    date=$(git log -1 --format=%cs "${cur}^{commit}")
    printf '%s\n' "$body" | sed "1s|^## \[.*|## [${cur}] - ${date}|"
    printf '\n'
}

# Oldest tag has no predecessor; bound its range at the repo root so git-cliff
# emits a closed range (an open-ended single ref emits nothing when the tag's own
# commit is path-filtered out).
ROOT="$(git rev-list --max-parents=0 HEAD | tail -1)"

# Decide where the version-being-cut section is sourced from.
#
# A PATCH ships only the commits cherry-picked onto its release branch -- NOT
# everything on main since the last tag, because main interleaves fixes bound for
# this line with features bound for the next minor (e.g. main carries a breaking
# api change that must not appear in a 0.17 patch). So for a patch we source the
# new section from release/v<major>.<minor>.x rather than HEAD. That lets you run
# this from any branch (main included) and still get exactly what the patch ships.
#
# A minor/major has no backport branch to read from, so it falls through to the
# normal "commits after the latest tag, walked from HEAD" behaviour.
CUT_RANGE=""
CUT_REF=""
if [[ -n "$NEXT_VERSION" ]]; then
    next_mm="${NEXT_VERSION%.*}" # v0.17.1 -> v0.17
    prev_same_line=$(printf '%s\n' "${TAGS[@]}" |
        grep -E "^${COMPONENT}/${next_mm//./\\.}\." | tail -1 || true)
    if [[ -n "$prev_same_line" ]]; then
        for _cand in "release/${next_mm}.x" "origin/release/${next_mm}.x"; do
            if git rev-parse --verify --quiet "${_cand}^{commit}" >/dev/null; then
                CUT_REF="$_cand"
                break
            fi
        done
        if [[ -z "$CUT_REF" ]]; then
            echo "ERROR: cutting patch ${COMPONENT}/${NEXT_VERSION}, but no release branch" >&2
            echo "       release/${next_mm}.x (or origin/release/${next_mm}.x) exists." >&2
            echo "       Create it and cherry-pick the fix(es) onto it first." >&2
            echo "       See docs/release-process.md (Patch Release Workflow)." >&2
            exit 1
        fi
        CUT_RANGE="${prev_same_line}..${CUT_REF}"
        echo "${C_DIM}Patch cut: sourcing ${COMPONENT}/${NEXT_VERSION} from ${CUT_REF} (${CUT_RANGE})${C_RESET}" >&2
    fi
fi

{
    printf '# Changelog\n\n'
    printf '<!-- DO NOT EDIT. Generated from git commit history by scripts/gen-changelog.sh.\n'
    printf '     Hand-authored behavior/upgrade notes live in RELEASE_NOTES.md (same directory). -->\n\n'
    printf 'All notable changes to this project will be documented in this file.\n\n'

    # Unreleased (= commits after the latest tag), or the version being cut.
    if [[ -n "$NEXT_VERSION" && -n "$CUT_RANGE" ]]; then
        # Patch: source only what's on the release branch since this line's last
        # tag (set above). Explicit range, not --unreleased, so the source is the
        # release branch rather than HEAD. No tag exists yet, so stamp the heading
        # with the release branch tip's date.
        # shellcheck disable=SC2086
        cut_body=$(git-cliff "${CLIFF_COMMON[@]}" --tag "${COMPONENT}/${NEXT_VERSION}" $CUT_RANGE 2>/dev/null) || true
        if [[ -z "${cut_body//[[:space:]]/}" ]]; then
            echo "WARNING: no ${COMPONENT} commits for ${NEXT_VERSION} in ${CUT_RANGE} -- cherry-picked onto ${CUT_REF} yet?" >&2
        else
            cut_date=$(git log -1 --format=%cs "${CUT_REF}^{commit}")
            printf '%s\n' "$cut_body" | sed "1s|^## \[.*|## [${COMPONENT}/${NEXT_VERSION}] - ${cut_date}|"
            printf '\n'
        fi
    elif [[ -n "$NEXT_VERSION" ]]; then
        # A fresh minor/major: --tag labels the unreleased commits; today's date is correct.
        git-cliff "${CLIFF_COMMON[@]}" --unreleased --tag "${COMPONENT}/${NEXT_VERSION}" 2>/dev/null
        printf '\n'
    else
        git-cliff "${CLIFF_COMMON[@]}" --unreleased 2>/dev/null || true
        printf '\n'
    fi

    # Released sections, newest -> oldest. Ranges are bounded by adjacent final
    # tags so each call yields exactly one section.
    for ((i = ${#TAGS[@]} - 1; i >= 0; i--)); do
        cur="${TAGS[i]}"
        if [[ $i -gt 0 ]]; then
            released_section "$cur" "${TAGS[i - 1]}..${cur}"
        else
            released_section "$cur" "${ROOT}..${cur}" # oldest final tag: root -> tag
        fi
    done

    printf '<!-- Generated by git-cliff -->\n'
} >"$OUTPUT"

echo "${C_GREEN}Updated ${OUTPUT}${C_RESET} (${#TAGS[@]} releases${NEXT_VERSION:+ + ${NEXT_VERSION}})"
