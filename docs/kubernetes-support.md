# Kubernetes Version Support

This document outlines Skyhook's approach to supporting different Kubernetes versions.

## What Version Should I Use?

The operator relies only on core, long-stable Kubernetes APIs (nodes, pods, configmaps, taints, cordon/drain) and gates on no version-specific features. It therefore very likely runs on clusters well older than the tested set, plausibly back to around 1.23, though those older versions are not CI-tested and so not officially supported.

What we **CI-test and officially support is the latest four Kubernetes minor versions** (see [Support Policy](#support-policy)).

| Kubernetes Version | Status |
|--------------------|--------|
| 1.36, 1.35, 1.34, 1.33 | ✅ Supported and CI-tested |
| ~1.23 – 1.32 | 🟡 Untested but expected to work (core APIs only) |
| Older than ~1.23 | ❌ Unverified |

## Support Policy

**Latest-four rolling window:** we test and support the **four most recent Kubernetes minor versions**. As a new minor release arrives and [kind](https://kind.sigs.k8s.io/) stops publishing a `kindest/node` image for the oldest one, we drop the oldest and add the newest. The exact tested patch versions live in `operator/versions.yaml` (`ci.kindNodeImages`), and the CI matrix is bounded by the node images the pinned kind (`kind.binary`) actually publishes.

Currently tested: **1.36, 1.35, 1.34, 1.33** (kind v0.32.0). 1.32 and older dropped when kind v0.32.0 stopped publishing their node images.

### Our Strategy

- **Support the latest four Kubernetes minor versions**, bounded by kind's published `kindest/node` images
- **Wait 4+ weeks** before adopting brand new Kubernetes versions (let them stabilize)
- **Older releases** remain available for users on older Kubernetes clusters
- **Clear compatibility** - each release has a defined K8s support window

### What This Means

- **✅ Fully Supported:** We test and support these K8s versions in the current Skyhook release
- **⚠️ Use older Skyhook:** Your K8s version is supported, but use an older Skyhook release
- **❌ Not Supported:** Upgrade your Kubernetes cluster or use a much older Skyhook version

### When Versions Change

**For new Kubernetes releases:**

1. Wait **4+ weeks** after K8s release for ecosystem stability
2. Add to the CI testing matrix in `operator/versions.yaml`
3. Include in next Skyhook release

**For EOL Kubernetes versions:**

1. Stop including in new Skyhook releases
2. Existing Skyhook versions continue to work
3. Users should upgrade K8s and then upgrade Skyhook

## Upgrade Strategy

### Our Approach

- Update Kubernetes client libraries when we add support for new versions
- Test on both supported Kubernetes versions before each release
- Provide clear migration guidance when dropping version support

### For Users

We understand many installations run slightly older Kubernetes versions. Our strategy balances staying current while giving users time to upgrade:

- **6-week notice** before dropping support for a Kubernetes version
- **Clear documentation** about which Skyhook version to use for your Kubernetes version
- **Gradual transitions** rather than sudden jumps when possible

## Version Selection Guide

Use the **latest release**. It is CI-tested against the latest four Kubernetes minor versions (currently 1.33 through 1.36) and, because it depends only on core Kubernetes APIs, is expected to run on older clusters (roughly back to 1.23) without CI coverage.

If your cluster is older than the tested range, the operator will very likely still run; upgrade into 1.33 – 1.36 when you can for the fully supported, CI-tested experience.

## FAQ

### Why don't you support EOL Kubernetes versions in new releases?

As a small project, we focus our efforts on actively maintained Kubernetes versions. This allows us to:

- Ensure better quality and security
- Adopt new Kubernetes features when they're stable  
- Keep our testing matrix manageable
- Provide clearer upgrade paths

### What if I'm stuck on an older Kubernetes version?

**You can still use Skyhook!** Just use an older Skyhook version that was built for your K8s version:

- Older releases continue to work and don't disappear
- Check our release notes for which Skyhook version supports your K8s version
- Plan your Kubernetes upgrade timeline, then upgrade Skyhook afterward

### Why wait 4 weeks before supporting new Kubernetes versions?

We've learned that brand new Kubernetes versions often have:

- Ecosystem compatibility issues
- Updated client library dependencies  
- Undiscovered bugs that get fixed in patch releases

Waiting 4+ weeks lets the ecosystem stabilize and gives us confidence in supporting the new version.

### How do you test compatibility?

For each Skyhook release, we test against all supported Kubernetes versions using:

- GitHub Actions matrix builds with multiple K8s versions. The exact tested patch versions are owned by `ci.kindNodeImages` in `operator/versions.yaml`.
- Local testing with [kind](https://kind.sigs.k8s.io/)
- Basic functionality and integration tests

## Notes

This is a living document that will evolve as the project grows. Our current approach supports all actively maintained Kubernetes versions (the latest four minor versions) while providing reasonable predictability for users.

For questions about Kubernetes support, please open an issue in our [GitHub repository](https://github.com/NVIDIA/skyhook).
