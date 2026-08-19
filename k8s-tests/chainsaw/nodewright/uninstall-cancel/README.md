<!--
SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
SPDX-License-Identifier: Apache-2.0

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
-->

# Uninstall Cancel Test

## Purpose

Verifies cancellation semantics: flipping `uninstall.apply: true` → `false`
while the package is mid-uninstall (at `StageUninstall`) resets the package
back into the install pipeline, and the node returns to `complete` with the
package re-installed. The webhook permits the transition with a warning, not
a rejection.

Cancellation is **uncancellable** once the package has reached
`StageUninstallInterrupt` — that edge case is not covered here; this test
targets the reversible window.

## Test Scenario

1. Install the package with `uninstall.enabled: true`, wait for complete.
2. Flip `uninstall.apply: true`. Wait for the NodeWright label to show
   `status_<name>: in_progress` (uninstall has started).
3. Sleep 15s so the uninstall pod has time to be running (keeping the test
   in the cancellable window before the interrupt phase).
4. Flip `uninstall.apply: false` via the cancel patch.
5. Assert the node returns to `status_<name>: complete` and `nodeState`
   shows the package back at `stage=config, state=complete`.
6. Assert the NodeWright is `status: complete`.
7. Flip `uninstall.apply: true` again and wait for the package to be absent
   from node state. This is purely operational — it leaves the CR with no
   installed packages so chainsaw's automatic CLEANUP can delete it without
   waiting on the delete-time uninstall finalizer, and incidentally covers
   that a cancelled-then-reinstalled package can subsequently be uninstalled.

## Key Features Tested

- `HandleCancelledUninstalls` path: StageUninstall → reset to install
  pipeline
- Package re-installs cleanly after cancel
- NodeWright returns to `complete` without manual intervention
- Webhook accepts the `apply: true` → `false` transition (warning, not
  rejection)

## Files

- `chainsaw-test.yaml` — Main test: install → trigger → cancel → reinstall → uninstall
- `nodewright.yaml` — Initial NodeWright, `uninstall.enabled: true`
- `update-trigger-uninstall.yaml` — Patch setting `uninstall.apply: true` (reused for the final uninstall step)
- `update-cancel-uninstall.yaml` — Patch setting `uninstall.apply: false`
