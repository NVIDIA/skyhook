<!--
  SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
  SPDX-License-Identifier: Apache-2.0
-->

# events.k8s.io RBAC test

Regression guard for [#308](https://github.com/NVIDIA/nodewright/issues/308).

The operator records events through the `events.k8s.io/v1` API (client-go
`tools/events`, wired via `mgr.GetEventRecorder`). The deployed manager
`ClusterRole` must therefore grant `create`/`patch` on `events.events.k8s.io` —
granting only the legacy core (`""`) events API causes every recorded event to
be rejected as `forbidden` and dropped.

This test installs the Helm chart, resolves the operator `ServiceAccount` from
the manager `Deployment`, and asserts via `kubectl auth can-i --as` that the SA
may both `create` and `patch` `events.events.k8s.io` (the two verbs the rule
grants — `create` for new events, `patch` for the aggregated series). A negative
control (`delete nodes`, which the role does not grant) confirms the impersonated
check discriminates rather than passing unconditionally.

Because it validates chart-rendered RBAC, it lives in the `helm` suite (the
`nodewright` e2e suite runs the operator via `make run` with the developer's
kubeconfig, which bypasses the ServiceAccount's RBAC entirely).
