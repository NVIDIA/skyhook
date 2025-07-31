# Skyhook Release Process

Step-by-step process for releasing Skyhook components using **release branches**.

## Release Branch Strategy

Skyhook uses **release branches** (`release/v{MAJOR.MINOR}.x`) to manage integrated releases and patches.

**Key Principles:**
- **Operator-centric**: Most releases are driven by operator features and bug fixes
- **Agent follows**: Agent changes typically only require chart patch releases 
- **Chart coordinates**: Chart version tracks the overall release and defines compatibility

### Major/Minor Release Workflow

```bash
# 1. Complete development on main
git checkout main && git pull origin main
# Ensure all features/fixes are merged and tested

# 2. Create release branch
git checkout -b release/v0.9.x
git push origin release/v0.9.x

# 3. Update chart with final versions
# Edit chart/Chart.yaml:
version: v0.9.0        # Chart version
appVersion: v0.9.0     # Recommended operator version

git add chart/Chart.yaml
git commit -m "release: prepare v0.9.0"
git push origin release/v0.9.x

# 4. Tag all components that changed  
git tag operator/v0.9.0    # Operator drives the release
git tag agent/v6.4.0       # Only if agent changed (often reuses previous version)
git tag chart/v0.9.0       # Chart always gets tagged
git push origin operator/v0.9.0 chart/v0.9.0  # Push operator + chart (add agent tag if needed)
```

**Automated:** Tests → Multi-platform build → Publish to ghcr.io + nvcr.io + NGC

### Patch Release Workflow

```bash
# 1. Work on release branch
git checkout release/v0.9.x
git pull origin release/v0.9.x

# 2. Apply fixes (backport from main or develop directly)
# ... make changes to operator, agent, or chart
git add .
git commit -m "fix: critical security issue"

# 3. Update chart version if needed
# Edit chart/Chart.yaml:
version: v0.9.1        # Increment patch version
appVersion: v0.9.1     # Update if operator changed

# 4. Tag only what changed
git tag operator/v0.9.1    # If operator changed
git tag agent/v6.4.1       # Only if agent changed (rare)
git tag chart/v0.9.1       # Chart always gets tagged for releases
git push origin operator/v0.9.1 chart/v0.9.1  # Usually just operator + chart
```

### Agent-Only Changes

```bash
# Agent changes typically don't require new release branches
git checkout release/v0.9.x  # Work on existing release branch
# ... fix agent issue
git tag agent/v6.4.1         # New agent version
git tag chart/v0.9.1         # Patch chart to reference new agent
git push origin agent/v6.4.1 chart/v0.9.1
```

### Legacy: Individual Component Releases (Deprecated)

*The following workflows are deprecated in favor of the release branch strategy above.*

<details>
<summary>Click to expand legacy workflows</summary>

#### Operator Release (Legacy)
```bash
git checkout main && git pull origin main
git tag operator/v1.2.3
git push origin operator/v1.2.3
```

#### Agent Release (Legacy)
```bash
git checkout main && git pull origin main
git tag agent/v1.2.3
git push origin agent/v1.2.3
```

#### Chart Release (Legacy)
```bash
git checkout -b release/chart-v1.2.3
# Update Chart.yaml, create PR, merge
git checkout main && git pull origin main
git tag chart/v1.2.3
git push origin chart/v1.2.3
```

</details>

## Release Checklist

**Before tagging:**
- [ ] All PRs merged to main
- [ ] For charts: Chart.yaml updated and merged
- [ ] Tests passing
- [ ] Documentation updated

**After tagging:**
- [ ] CI/CD pipeline completes
- [ ] Images published successfully
- [ ] Test deployment with new version

## Common Commands

```bash
# Check current tags
git tag -l 'operator/v*' --sort=-v:refname | head -5
git tag -l 'agent/v*' --sort=-v:refname | head -5  
git tag -l 'chart/v*' --sort=-v:refname | head -5

# See what will be included in tag
git log --oneline $(git tag -l 'operator/v*' --sort=-v:refname | head -1)..HEAD

# Delete tag if needed (before CI runs)
git tag -d operator/v1.2.3
git push origin :refs/tags/operator/v1.2.3
```

## Rollback

For problematic releases:
1. Tag new patch release with fixes
2. For critical issues: Update chart `appVersion` to previous stable version

See [versioning.md](versioning.md) for version strategy details. 