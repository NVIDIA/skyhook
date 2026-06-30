# NodeWright Roadmap

This roadmap tracks the remaining work toward NodeWright v1 (GA) and the enhancements planned beyond it. It is the cleaned, decision-bearing form of the v1 framework reviewed with the team on 2026-06-01.

## Objectives

v1 is the first release we declare stable and production-ready. It is a promise, not a feature count: the API is stable, the core node-lifecycle use cases work end-to-end, it is safe to run unattended on a real fleet, and an outside team can adopt it without us in the room. We sort every candidate against four objectives, and an item only blocks v1 if it fails one of them.

| Objective | Definition |
|-----------|------------|
| **API Stability** | The CRDs graduate `v1alpha1 → v1` under the `nodewright` group, with a frozen schema, a conversion path, and a documented policy for making breaking changes after GA. |
| **Feature Completeness** | The core lifecycle is fully covered with no "fork the agent" gaps: packages as OCI artifacts, a Go agent, package-declared interrupts, and dynamic cordon/drain. |
| **Production Hardening** | Safe to run unattended: no unmaintained or unversioned third-party images, correct behavior under partial failure, verifiable provenance, and CI that is green without reruns. |
| **Adoption Readiness** | An outside team can install, author a package, deploy, and upgrade using only public docs and the CLI. |

## Scope

### 1. API Stability

**Rename to the `nodewright.nvidia.com` API group (required).** The Go module, Helm chart, and operator image are already `nodewright`; the CRD group (`skyhook.nvidia.com/v1alpha1`), CLI, and namespace still say `skyhook`. The API group name is the single least-reversible thing after v1, so the rename resolves before the freeze, not as a later concern. Existing `skyhook.nvidia.com/v1alpha1` resources get a conversion path.

**Graduate the CRDs to `v1`.** Lock the Skyhook and DeploymentPolicy schemas and ship a conversion webhook so existing clusters migrate in place.

**Freeze the package contract.** What the operator and CLI can know about a package changes fundamentally once packages are OCI artifacts, so the package contract is part of the surface we are freezing. The schema-affecting work that must land before the freeze: configurable node-drain options ([#259](https://github.com/NVIDIA/nodewright/issues/259)), admission-webhook validation of the `dependsOn` DAG (acyclic, every target present, versions well-formed), package-declared interrupts, and the new preflight stage. Dependency handling is validation only: there is no resolver and no auto-install of dependencies in v1.

**Feature-release process.** A defined way to introduce and graduate features after v1 so new work lands without destabilizing the frozen surface. The likely shape is a maturity path every feature flows through (off-by-default or opt-in before it becomes default), with feature flags as one mechanism for opting in or out. The process is the deliverable, not just the flags; because the flags surface is most likely a CRD field or operator config, its shape is part of what we freeze. This is the "how do we add things safely after the freeze" counterpart to the versioning policy below.

**Versioning and compatibility policy.** Defining how we change the API safely after v1 is itself a v1 deliverable. Document the per-surface major-bump policy, establish a deprecation channel that warns before it breaks, and wire compatibility tests into CI so unintended breakage is caught.

**Acceptance:** `v1.0.0` ships the `nodewright.nvidia.com/v1` group with a frozen, documented schema and package contract, a conversion path from `v1alpha1`, and a CI-enforced compatibility policy that gates any breaking change behind a major-version bump.

### 2. Feature Completeness

**The shared core: Go agent and OCI/ORAS packages.** Completing the Python-to-Go agent rewrite and moving packages to OCI artifacts ([#194](https://github.com/NVIDIA/nodewright/issues/194), [#214](https://github.com/NVIDIA/nodewright/issues/214)–[#222](https://github.com/NVIDIA/nodewright/issues/222)) is the keystone of v1. Today the Python agent is a black box the operator can only run and watch. Once the agent is Go and packages are OCI artifacts, package-handling becomes one shared Go library that all three binaries use, each in its own role: the operator reads a package to drive orchestration (its steps, interrupts, uninstall support, schema, dependencies) without executing it; the CLI uses the same library to validate a package locally before it is applied; the agent streams and applies the layers.

This cutover must complete before GA. Shipping v1 on the Python agent and swapping the runtime to Go afterward would be too disruptive for users, so the cutover is a v1 blocker rather than a 1.x follow-up.

**Bring-your-own, discovered interrupts.** Interrupts stop being a fixed enum the operator and agent both hardcode (NoOp, NodeRestart, ServiceRestart). A package declares its own interrupt, and the operator discovers what a package needs by reading it. The package becomes the source of truth; the operator orchestrates whatever it finds. Interrupts configured in the spec do not go away: they become an **override** of the discovered behavior, so an operator can force or suppress an interrupt when they need to deviate from what the package reports.

**Preflight stage and dynamic cordon/drain.** A new `preflight` lifecycle stage runs before cordon/drain, where each package reports whether it actually needs to interrupt this time. Cordon and drain become dynamic: the operator skips them when nothing requires an interrupt, rather than cordoning and draining because a package might. Because preflight evaluates what would happen without doing it, running it in report-only mode is the natural substrate for dry-run.

**The CLI as an operations tool.** The shared library that reads packages powers client-side validation: validate a package, show "what will this do", and diff versions before anything touches a node. That validation and the operational verbs are in scope for v1: fine-grained node-state edits and targeted reset ([#257](https://github.com/NVIDIA/nodewright/issues/257)).

**Acceptance:** a package distributed as a signed OCI artifact is inspected by the operator and applied by the Go agent end-to-end, and a node is cordoned and drained only when a package's preflight reports that an interrupt is required.

### 3. Production Hardening

**Replace unmaintained and unversioned dependencies.** Move operator metrics off kube-rbac-proxy to controller-runtime's built-in auth ([#206](https://github.com/NVIDIA/nodewright/issues/206)), which also resolves the TLS-handshake noise reports, and migrate the cleanup-job image off `bitnami/kubectl` to a maintained, versioned alternative ([#207](https://github.com/NVIDIA/nodewright/issues/207)).

**Correctness under partial failure.** Stop the ConfigMap volume from clobbering files baked into the package image ([#208](https://github.com/NVIDIA/nodewright/issues/208)); fix the ConfigMap desync where a stale `Status.ConfigUpdates` entry drives a spurious interrupt ([#245](https://github.com/NVIDIA/nodewright/issues/245)); fix the taint-on-reboot case when `applyOnReboot`, `runtimeRequired`, and `autoTaintNewNodes` combine ([#180](https://github.com/NVIDIA/nodewright/issues/180)); and surface ImagePullBackOff / ErrImagePull as an explicit error state instead of a silent hang.

**Run package execution as Jobs.** Migrate package execution from raw Pods to Kubernetes Jobs ([#223](https://github.com/NVIDIA/nodewright/issues/223)) so retry and backoff use native semantics.

**Verifiable provenance.** Build-time provenance already ships. v1 adds the runtime half: the operator and CLI verify a package's attestation via the OCI referrers API before pull or apply. This lands together with the OCI package work rather than as a separate path.

**Release and CI reliability.** Eliminate the false-positive e2e flakiness that forces reruns, and fix the bug where RC tags steal release notes from the stable release ([#246](https://github.com/NVIDIA/nodewright/issues/246)).

**Multi-tenant node access control (design-first).** The operator is a privileged deputy: at apply time, node mutations run with the operator's credentials, so RBAC on the namespaced Skyhook CR does not constrain which cluster-scoped nodes a tenant can target. Close this with admission-time authorization keyed on the requester's identity. This needs a design before implementation, and its v1 gating depends on whether multi-tenant operation is a v1 use case.

**Acceptance:** the operator runs unattended on a real fleet with no unmaintained or unversioned third-party images; package execution retries through Jobs; error states such as image-pull failures and stale-status drift surface as status and events rather than hangs; every package is provenance-verified before apply; and CI passes without reruns.

### 4. Adoption Readiness

**Documentation and examples.** A completeness pass over install, upgrade, and day-2 operations so a new adopter is never blocked on tribal knowledge.

**CLI distribution and compatibility.** The CLI is distributed independently of the operator and must work against any supported operator version. Keep the CLI-to-operator backward-compatibility matrix current and feature-detect rather than silently no-op.

**Project hygiene.** Add issue and PR-management automation (triage, labeler, welcome, lock-threads, inactive-PR reminders) ([#260](https://github.com/NVIDIA/nodewright/issues/260)) so the contribution pipeline scales with inbound flow.

**Install and upgrade UX.** Review the install and upgrade flow end-to-end so adoption does not require hand-holding.

**Acceptance:** an outside team installs NodeWright, authors and deploys a package, and upgrades across a release using only public docs and the CLI, without contacting the maintainers.

## Revision History

| Date | Change |
|------|--------|
| 2026-06-02 | Initial roadmap drafted from the v1 framework and the 2026-06-01 team review |
