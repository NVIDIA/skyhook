# Kubernetes Version Support

This document outlines Skyhook's approach to supporting different Kubernetes versions.

## Current Support Matrix

| Kubernetes Version | Skyhook Version | Status | Notes |
|--------------------|-----------------|---------|-------|
| 1.35, 1.34, 1.33, 1.32, 1.31  | v0.11.0+         | ✅ Next Supported | Current stable versions |
| 1.34, 1.33, 1.32, 1.31  | v0.9.0 - 0.10.0        | ✅ Fully Supported | Current stable versions |
| 1.30               | v0.8.x          | ⚠️ Use older Skyhook | K8s 1.30 EOL: June 28, 2025 |
| 1.29 and older    | v0.8.x or older | ⚠️ Use older Skyhook | No longer maintained |

## Support Policy

**Release Window Approach:** Each Skyhook release supports the Kubernetes versions that were actively maintained (non-EOL) at the time of that release.

### Our Strategy

- **Support all current non-EOL Kubernetes versions** (typically 3 versions)
- **Wait 4+ weeks** before adopting brand new Kubernetes versions (let them stabilize)
- **Older Skyhook versions** remain available for users on older Kubernetes clusters
- **Clear compatibility** - each release has a defined K8s support window

### What This Means

- **✅ Fully Supported:** We test and support these K8s versions in the current Skyhook release
- **⚠️ Use older Skyhook:** Your K8s version is supported, but use an older Skyhook release
- **❌ Not Supported:** Upgrade your Kubernetes cluster or use a much older Skyhook version

### When Versions Change

**For new Kubernetes releases:**
1. Wait **4+ weeks** after K8s release for ecosystem stability
2. Add to our CI testing matrix
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

**Choose your Skyhook version based on your Kubernetes version:**

- **Kubernetes 1.34, 1.33, 1.32, or 1.31:** Use latest Skyhook (v0.9.x or v0.10.0)
- **Kubernetes 1.30:** Use Skyhook v0.8.x (K8s 1.30 is EOL but v0.8.x still works)
- **Kubernetes 1.29 or older:** Use Skyhook v0.8.x or older (check release notes for compatibility)

### Migration Path

**If you're on an older Kubernetes version:**
1. **First:** Upgrade your Kubernetes cluster to a supported version (1.31, 1.32, 1.33, or 1.34)
2. **Then:** Upgrade to the latest Skyhook version

**If you're on Kubernetes 1.30:**
- **Option A:** Upgrade to K8s 1.31/1.32/1.33/1.34, then use latest Skyhook
- **Option B:** Stay on Skyhook v0.8.x until you can upgrade Kubernetes

**Recommended:** If you can choose your Kubernetes version, use 1.34, 1.33, or 1.32 for the longest support runway.

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
- GitHub Actions matrix builds with multiple K8s versions (currently 1.31, 1.32, 1.33, 1.34)
- Local testing with [kind](https://kind.sigs.k8s.io/)
- Basic functionality and integration tests

## Notes

This is a living document that will evolve as the project grows. Our current approach supports all actively maintained Kubernetes versions (typically 3 versions) while providing reasonable predictability for users.

For questions about Kubernetes support, please open an issue in our [GitHub repository](https://github.com/NVIDIA/skyhook). 