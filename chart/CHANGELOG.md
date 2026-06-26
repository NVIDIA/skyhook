# Changelog

<!-- DO NOT EDIT. Generated from git commit history by scripts/gen-changelog.sh.
     Hand-authored behavior/upgrade notes live in RELEASE_NOTES.md (same directory). -->

All notable changes to this project will be documented in this file.

## [chart/v0.17.1] - 2026-06-26

### Bug Fixes

- *(chart)* Repair immutable Deployment selector on skyhook->nodewright upgrade  

## [chart/v0.17.0] - 2026-06-12

### Bug Fixes

- *(chart)* Agent container path pointing to skyhook not nodewright

### New Features

- *(changelog)* Tag-range generator, split CHANGELOG/RELEASE_NOTES, release-tag helper 

### Other Tasks

- *(docs)* Update docs around release and location of helm chart 
- *(chart)* Bump to v0.16.0 with pinned operator and agent digests
- Merge pull request #247 from NVIDIA/chore/chart-bump-v0.16.0

chore(chart): bump to v0.16.0 with pinned operator and agent digests
- Merge pull request #250 from NVIDIA/fix/chart

fix(chart): agent container path pointing to skyhook not nodewright
- Merge pull request #255 from NVIDIA/chore/chart-bump-v0.16.1

chore(chart): bump to v0.16.1 with operator webhook cert deadlock fix
- Bump chart versions  

## [chart/v0.16.1] - 2026-05-26

### Other Tasks

- Merge pull request #255 from NVIDIA/chore/chart-bump-v0.16.1

chore(chart): bump to v0.16.1 with operator webhook cert deadlock fix
- Merge pull request #256 from NVIDIA/feat/cherry-pick

Feat: cherry pick

## [chart/v0.16.0] - 2026-05-22

### Bug Fixes

- Update helm chart for drift 

### New Features

- Pick to 16 

### Other Tasks

- Run helm tests with ctlptl registry 
- Update go and libs to latest 
- Parallelize e2e tests by pool and add merge gates 
- *(docs)* Update docs around release and location of helm chart  
- *(chart)* Bump to v0.16.0 with pinned operator and agent digests
- Merge pull request #248 from NVIDIA/cherry-pick/chart-to-v0.16.0

chore(chart): bump to v0.16.0 with pinned operator and agent digests

## [chart/v0.15.1] - 2026-04-14

### Other Tasks

- *(chart)* Update min k8s version in chart

## [chart/v0.15.0] - 2026-04-06

### New Features

- Add SKYHOOK_NODE_ORDER env var for monotonic node ordering

### Other Tasks

- Update project to follow the OSS template
- *(chart)* Version bump

## [chart/v0.14.0] - 2026-03-10

### New Features

- Add sequencing: node or all

### Other Tasks

- Update chart versions

## [chart/v0.13.1] - 2026-03-04

### Other Tasks

- Version bump 

## [chart/v0.13.0] - 2026-03-03

### Bug Fixes

- Resolve webhook caBundle deadlock during helm upgrade

### New Features

- AutoTaintNewNodes

### Other Tasks

- Chart version bump  
- Update chart with versions 

## [chart/v0.12.1] - 2026-02-10

### Bug Fixes

- Resolve webhook caBundle deadlock during helm upgrade

### Other Tasks

- Chart version bump 

## [chart/v0.12.0] - 2026-02-06

### Bug Fixes

- Make imagePullSecret optional to prevent kubelet errors

### New Features

- Add new printer columns
- *(chart)* Add automatic Skyhook resource cleanup on helm uninstall
- *(deployment-policy)* Add batch state reset with auto-reset, CLI, and config

### Other Tasks

- *(chart)* Update versions
- *(chart)* Update versions 

## [chart/v0.11.1] - 2026-01-12

### Other Tasks

- Version bump

## [chart/v0.11.0] - 2025-12-24

### Bug Fixes

- *(chart)* Add missing rbac for deploymentpolicies
- Sync chart CRD deploymentPolicy type and add smoke test 
- Un namespace policies
- Bad webhook rules

### Other Tasks

- Update release values
- Update docs and chart versions

## [chart/v0.10.1] - 2025-12-22

### Bug Fixes

- *(chart)* Add missing rbac for deploymentpolicies
- Sync chart CRD deploymentPolicy type and add smoke test 

### Other Tasks

- Update release values

## [chart/v0.10.0] - 2025-12-04

### Bug Fixes

- *(chart)* Resolve kubernetes security scan violations for compliance 
- *(chart)* Use image tags instead of digest for multi-registry support 

### New Features

- *(crd)* Add deployment policy 
- Add DeploymentPolicy validation and defaults with tests 
- Implement deployment strategies with compartment-based batching 
- Compartment status 
- *(operator)* Update k8s version to 1.34.0 
- Add container sha as optional field to package 

### Other Tasks

- Update the chart k8s version, operator version, and agent version 
- Release 0.10.0 

## [chart/v0.9.2] - 2025-08-28

### Other Tasks

- Update agent version
- Update agent version
- Update chart k8s version requirement 

## [chart/v0.9.1] - 2025-08-25

### Other Tasks

- Update agent version

## [chart/v0.9.0] - 2025-08-08

### Bug Fixes

- *(chart)* Fix broken helm chart tests
- *(operator)* Make metrics binding disabled by default
- *(chart/metrics)* Update for prometheus auto scraping and rbac examples
- *(chart)* Set back to v6.1.4 agent due to bug in v6.2.0 

### New Features

- *(chart)* Enable scraping of metrics by prometheus
- *(operator)* Update k8s sdk version
- Fix agent for distroless and have scr name in flag/history/log 
- *(chart)* Add node affinity for operator pod configuration
- *(operator)* Added disabled, paused, waiting, and blocked statuses for skyhooks and nodes 

### Other Tasks

- *(helm)* Update versions

## [chart/v0.8.1] - 2025-08-01

### Other Tasks

- *(chart)* Update chart to use agent v6.3.0 and chart v0.8.1

## [chart/v0.8.0] - 2025-06-06

### Bug Fixes

- Remove interrupt timeout which was flawed by design
- Deadlock if reboot pods are missing, adds them back
- Miscellaneous fixes to project structure
- Race bug running more then one pod at a time
- Update tests to not set limits everywhere anymore
- How we compare interrupt pods
- Reviews
- *(operator)* Missed changes related to changing min value for priority

### New Features

- *(agent/ci)* Add unittest and coverage report job
- Change to common license formatter and update all code with that format
- Add gracefully shutdown support
- Remove cert manager
- Change how limits are manged to a use a limitrange via helm
- *(operator)* Add strict ordering of skyhooks along with documentation
- *(operator)* Change default resources to follow a 2:1 ratio and add documentation about scaling

### Other Tasks

- *(helm)* Added docs for the helm chart
- *(chart)* Update version to correct new version

<!-- Generated by git-cliff -->
