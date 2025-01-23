# Requirements
1. Release tag must be in the format `vMajor.Minor.Patch` generally all releases will be a Major or Minor. Patch is reserved for a release that fixes an already released Major or Minor version.

# How to make a release
1. Be absolutely sure the helm chart is how you want to release. We CANNOT delete helm chart releases from ngc.nvidia.com.
1. Run changelog generation from the last tag
    1. clone dgx/infra/tools
    1. `git log --no-merges ${LAST_RELEASE_TAG}...HEAD | python3 internal/scripts/make_changelog.py ${RELEASE_TAG}`
    1. Copy the output from the prior command and edit to remove and condense changes that would be not relevant for an outside reader. Examples of these are internal only changes like unit test or ci or e2e things.
1. Tag main branch with the release
    1. Set the message of the tag to be the changelog
1. Create a release of tag
    1. Set the description of the release to be the changelog
1. Update any old releases if they have open issues that are fixed in this version
1. Create a branch using the system outlined in the Branch Strategy section

# Branch Strategy
We use long lived branches in order to be able to release patches for old versions. The strategy is as follows:
1. If this is a Major or Minor release create a branch to be able to put patches into. Examples:
    1. Release `v0.5.0` will have a branch `branch-v0.5.x` and any patch would be tagged from this branch for example `v0.5.1`
    1. Release `v1.0.0` will have branch `branch-v1.0.x`
    1. Release `v1.1.0` will have branch `branch-v1.1.x`