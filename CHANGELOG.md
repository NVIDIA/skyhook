<!--
  SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
  SPDX-License-Identifier: Apache-2.0
-->

# Changelog

Each Skyhook component is versioned and released independently.
Full changelogs are maintained per component and published as [GitHub Releases](https://github.com/NVIDIA/nodewright/releases).

## Components

| Component | Changelog |
|---|---|
| Operator | [operator/CHANGELOG.md](operator/CHANGELOG.md) |
| Agent | [agent/CHANGELOG.md](agent/CHANGELOG.md) |
| Helm Chart | [chart/CHANGELOG.md](chart/CHANGELOG.md) |
| CLI | [operator/cmd/cli/CHANGELOG.md](operator/cmd/cli/CHANGELOG.md) |

## Generating

Regenerate a component changelog from git history (interactive):

```bash
make changelog
```

Non-interactive form (e.g. for scripting or bulk refresh):

```bash
scripts/gen-changelog.sh operator
for c in operator agent chart cli; do scripts/gen-changelog.sh "$c"; done
```

See `docs/contributing/release-process.md` for the full changelog and release-tagging workflow.
