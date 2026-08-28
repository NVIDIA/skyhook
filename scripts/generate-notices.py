#!/usr/bin/env python3

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


"""Generate THIRD_PARTY_NOTICES.md for Skyhook (operator, agent, root rollup)."""

import json
import os
import re
import shutil
import subprocess
import sys
from datetime import datetime, timezone
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parent.parent
OPERATOR_DIR = REPO_ROOT / "operator"
AGENT_GO_DIR = REPO_ROOT / "agent" / "go"
AGENT_VENDOR = REPO_ROOT / "agent" / "vendor"
NOTICES_VENV = REPO_ROOT / "agent" / ".notices-venv"
LICENSES_CACHE = REPO_ROOT / ".licenses-cache"
OPERATOR_FILE = REPO_ROOT / "operator" / "THIRD_PARTY_NOTICES.md"
AGENT_FILE = REPO_ROOT / "agent" / "THIRD_PARTY_NOTICES.md"
ROOT_FILE = REPO_ROOT / "THIRD_PARTY_NOTICES.md"


def tag(prefix: str) -> str:
    r = subprocess.run(
        [
            "git",
            "-C",
            str(REPO_ROOT),
            "describe",
            "--tags",
            "--abbrev=0",
            "--match",
            f"{prefix}/v*",
        ],
        capture_output=True,
        text=True,
        check=False,
    )
    return r.stdout.strip() if r.returncode == 0 and r.stdout.strip() else "unreleased"


def now_utc() -> str:
    return datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def _collapse_blanks(text: str) -> str:
    """Collapse runs of 3+ newlines to exactly 2 (one blank line)."""
    return re.sub(r"\n{3,}", "\n\n", text)


def _go_license_rows(
    component_dir: Path, cache_dir: Path, extra_env: dict | None = None
):
    """Run go-licenses against a Go module and return sorted (pkg, url, license) rows.

    License text files for each package are saved under cache_dir/<pkg>/.
    """
    gl = component_dir / "bin" / "go-licenses"
    gl_path = str(gl) if gl.exists() else (shutil.which("go-licenses") or "")
    if not gl_path:
        sys.exit(
            f"ERROR: go-licenses not found. Run 'make -C {component_dir.relative_to(REPO_ROOT)} go-licenses'."
        )
    env = {**os.environ, **(extra_env or {})}
    stdlib = subprocess.check_output(
        ["go", "list", "std"], cwd=component_dir, env=env, text=True
    )
    stdlib_ignore = ",".join(
        sorted({line.split("/")[0] for line in stdlib.splitlines() if line})
    )
    local = subprocess.check_output(
        ["go", "list", "-m"], cwd=component_dir, env=env, text=True
    ).strip()
    ignore = f"{stdlib_ignore},{local}"

    if cache_dir.is_dir():
        shutil.rmtree(cache_dir)
    elif cache_dir.exists():
        cache_dir.unlink()
    subprocess.run(
        [
            gl_path,
            "save",
            "./...",
            f"--save_path={cache_dir}",
            "--force",
            f"--ignore={ignore}",
        ],
        cwd=component_dir,
        env=env,
        check=True,
    )
    csv = subprocess.check_output(
        [gl_path, "csv", "./...", f"--ignore={ignore}"],
        cwd=component_dir,
        env=env,
        text=True,
    )
    rows = sorted(
        {tuple(line.split(",", 2)) for line in csv.splitlines() if line.strip()}
    )
    if not rows:
        sys.exit(
            f"ERROR: go-licenses produced no entries for {component_dir.relative_to(REPO_ROOT)}."
        )
    return rows


def _render_go_section(heading: str, rows, cache_dir: Path) -> list[str]:
    out = [heading, "", "| Package | License | Source |", "|---|---|---|"]
    for pkg, url, lic in rows:
        out.append(f"| `{pkg}` | {lic or 'Unknown'} | {url or 'n/a'} |")
    out += ["", "## License Texts", ""]
    for pkg, url, lic in rows:
        out += [
            f"### {pkg}",
            "",
            f"* License: {lic or 'Unknown'}",
            f"* Source: {url or 'n/a'}",
            "",
        ]
        pkg_dir = cache_dir / pkg
        files = (
            sorted(p for p in pkg_dir.iterdir() if p.is_file())
            if pkg_dir.exists()
            else []
        )
        for f in files:
            out += [
                f"#### {f.name}",
                "",
                "```text",
                f.read_text(errors="replace").rstrip(),
                "```",
                "",
            ]
        if not files:
            out += [
                "License text unavailable. See upstream source for the full license.",
                "",
            ]
    return out


def operator_notices():
    rows = _go_license_rows(
        OPERATOR_DIR, LICENSES_CACHE / "operator", {"GOFLAGS": "-mod=vendor"}
    )
    out = [
        "# Third-Party Notices — Skyhook Operator + CLI",
        "",
        f"Generated: {now_utc()}",
        f"Operator tag: `{tag('operator')}`",
        "",
    ]
    out += _render_go_section("## Index", rows, LICENSES_CACHE / "operator")
    OPERATOR_FILE.write_text(_collapse_blanks("\n".join(out)) + "\n")
    print(
        f"Wrote {OPERATOR_FILE.relative_to(REPO_ROOT)} ({len(rows)} Go deps)",
        file=sys.stderr,
    )


def _agent_python_section():
    deps = []
    if AGENT_VENDOR.exists():
        for d in sorted(AGENT_VENDOR.iterdir()):
            if d.is_dir() and "-" in d.name:
                name, _, version = d.name.rpartition("-")
                deps.append((name, version))
    if not deps:
        return ["_No vendored Python dependencies found._", ""], 0

    if not (NOTICES_VENV / "bin" / "pip").exists():
        subprocess.run(["python3", "-m", "venv", str(NOTICES_VENV)], check=True)
    pip = str(NOTICES_VENV / "bin" / "pip")
    pip_licenses = str(NOTICES_VENV / "bin" / "pip-licenses")
    subprocess.run(
        [pip, "install", "--quiet", "--upgrade", "pip", "pip-licenses"], check=True
    )
    subprocess.run(
        [pip, "install", "--quiet", "--upgrade", *[f"{n}=={v}" for n, v in deps]],
        check=True,
    )
    raw = subprocess.check_output(
        [
            pip_licenses,
            "--packages",
            *[n for n, _ in deps],
            "--with-license-file",
            "--with-urls",
            "--format=json",
            "--no-license-path",
        ],
        text=True,
    )
    entries = sorted(json.loads(raw), key=lambda e: e["Name"].lower())

    def src(e) -> str:
        u = e.get("URL") or ""
        return u if u and u != "UNKNOWN" else "n/a"

    out = ["| Package | Version | License | Source |", "|---|---|---|---|"]
    for e in entries:
        out.append(
            f"| `{e['Name']}` | {e['Version']} | {e.get('License') or 'Unknown'} | {src(e)} |"
        )
    out += ["", "### License Texts", ""]
    for e in entries:
        out += [
            f"#### {e['Name']} {e['Version']}",
            "",
            f"* License: {e.get('License') or 'Unknown'}",
            f"* Source: {src(e)}",
            "",
        ]
        text = (e.get("LicenseText") or "").strip()
        if text:
            out += ["```text", text, "```", ""]
        else:
            out += [
                "License text unavailable. See upstream source for the full license.",
                "",
            ]
    return out, len(entries)


def _agent_go_section():
    go_cache = LICENSES_CACHE / "agent-go"
    rows = _go_license_rows(AGENT_GO_DIR, go_cache)
    out = ["| Package | License | Source |", "|---|---|---|"]
    for pkg, url, lic in rows:
        out.append(f"| `{pkg}` | {lic or 'Unknown'} | {url or 'n/a'} |")
    out += ["", "### License Texts", ""]
    for pkg, url, lic in rows:
        out += [
            f"#### {pkg}",
            "",
            f"* License: {lic or 'Unknown'}",
            f"* Source: {url or 'n/a'}",
            "",
        ]
        pkg_dir = go_cache / pkg
        files = (
            sorted(p for p in pkg_dir.iterdir() if p.is_file())
            if pkg_dir.exists()
            else []
        )
        for f in files:
            out += [
                f"##### {f.name}",
                "",
                "```text",
                f.read_text(errors="replace").rstrip(),
                "```",
                "",
            ]
        if not files:
            out += [
                "License text unavailable. See upstream source for the full license.",
                "",
            ]
    return out, len(rows)


def agent_notices():
    py_section, py_count = _agent_python_section()
    go_section, go_count = _agent_go_section()

    out = [
        "# Third-Party Notices — Skyhook Agent",
        "",
        f"Generated: {now_utc()}",
        f"Agent tag: `{tag('agent')}`",
        "",
        "## Python Dependencies",
        "",
        *py_section,
        "## Go Dependencies",
        "",
        *go_section,
    ]
    AGENT_FILE.write_text(_collapse_blanks("\n".join(out)) + "\n")
    print(
        f"Wrote {AGENT_FILE.relative_to(REPO_ROOT)} "
        f"({py_count} Python deps, {go_count} Go deps)",
        file=sys.stderr,
    )


def demote_and_strip_h1(text: str) -> str:
    out, in_fence, dropped_h1 = [], False, False
    for line in text.splitlines():
        if line.startswith("```"):
            in_fence = not in_fence
            out.append(line)
            continue
        if not in_fence and not dropped_h1:
            if line.startswith("# "):
                dropped_h1 = True
                continue
            if not line.strip():
                continue
            dropped_h1 = True  # content before any H1 — keep what follows
        if not in_fence and line.startswith("#"):
            out.append("#" + line)
        else:
            out.append(line)
    return "\n".join(out)


def rollup():
    for f, hint in ((OPERATOR_FILE, "operator"), (AGENT_FILE, "agent")):
        if not f.exists():
            sys.exit(
                f"ERROR: {f.relative_to(REPO_ROOT)} does not exist. Run 'make notices-{hint}' first."
            )
    op = demote_and_strip_h1(OPERATOR_FILE.read_text())
    ag = demote_and_strip_h1(AGENT_FILE.read_text())
    out = [
        "# Third-Party Notices",
        "",
        "Combined third-party notices for Skyhook (operator, CLI, agent).",
        "",
        f"Generated: {now_utc()}",
        f"Operator tag: `{tag('operator')}`",
        f"Agent tag: `{tag('agent')}`",
        f"Chart tag: `{tag('chart')}`",
        "",
        "## Operator + CLI",
        "",
        op,
        "",
        "## Agent",
        "",
        ag,
        "",
    ]
    ROOT_FILE.write_text(_collapse_blanks("\n".join(out)))
    print(f"Wrote {ROOT_FILE.relative_to(REPO_ROOT)} (rollup)", file=sys.stderr)


def main():
    mode = sys.argv[1] if len(sys.argv) > 1 else "all"
    if mode == "operator":
        operator_notices()
    elif mode == "agent":
        agent_notices()
    elif mode == "rollup":
        rollup()
    elif mode == "all":
        operator_notices()
        agent_notices()
        rollup()
    else:
        sys.exit("Usage: generate-notices.py [operator|agent|rollup|all]")


if __name__ == "__main__":
    main()
