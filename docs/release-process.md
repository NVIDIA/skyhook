# Skyhook Release Process

Step-by-step process for releasing Skyhook components (operator, agent, chart).

## Release Workflow

### Operator Release

```bash
# 1. Test thoroughly, merge all PRs to main
# 2. Tag and push
git checkout main && git pull origin main
git tag operator/v1.2.3
git push origin operator/v1.2.3
```

**Automated:** Tests → Multi-platform build → Publish to ghcr.io + nvcr.io → Attestations

### Agent Release

```bash
# 1. Test agent compatibility, merge all PRs to main  
# 2. Tag and push
git checkout main && git pull origin main
git tag agent/v1.2.3
git push origin agent/v1.2.3
```

**Automated:** Tests → Multi-platform build → Publish to ghcr.io + nvcr.io

### Chart Release

```bash
# 1. Update Chart.yaml versions
# chart/Chart.yaml
version: v1.2.3        # Chart version
appVersion: v0.8.0     # Recommended operator version

# 2. Create PR and merge
git checkout -b release/chart-v1.2.3
git add chart/Chart.yaml
git commit -m "chart: bump version to v1.2.3"
git push origin release/chart-v1.2.3
# Review and merge PR

# 3. Tag after merge
git checkout main && git pull origin main
git tag chart/v1.2.3
git push origin chart/v1.2.3
```

**Automated:** Package Helm chart → Publish to chart repository (when implemented)

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