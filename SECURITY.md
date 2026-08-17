## Security

NVIDIA is dedicated to the security and trust of our software products and services, including all source code repositories managed through our organization.

If you need to report a security issue, please use the appropriate contact points outlined below. **Please do not report security vulnerabilities through GitHub.** If a potential security issue is inadvertently reported via a public issue or pull request, NVIDIA maintainers may limit public discussion and redirect the reporter to the appropriate private disclosure channels.

## Reporting Potential Security Vulnerability in an NVIDIA Product

To report a potential security vulnerability in any NVIDIA product:
- Web: [Security Vulnerability Submission Form](https://www.nvidia.com/object/submit-security-vulnerability.html)
- E-Mail: psirt@nvidia.com
    - We encourage you to use the following PGP key for secure email communication: [NVIDIA public PGP Key for communication](https://www.nvidia.com/en-us/security/pgp-key)
    - Please include the following information:
   	 - Product/Driver name and version/branch that contains the vulnerability
     - Type of vulnerability (code execution, denial of service, buffer overflow, etc.)
   	 - Instructions to reproduce the vulnerability
   	 - Proof-of-concept or exploit code
   	 - Potential impact of the vulnerability, including how an attacker could exploit the vulnerability

While NVIDIA currently does not have a bug bounty program, we do offer acknowledgement when an externally reported security issue is addressed under our coordinated vulnerability disclosure policy. Please visit our [Product Security Incident Response Team (PSIRT)](https://www.nvidia.com/en-us/security/psirt-policies/) policies page for more information.

## Coordinated Disclosure

PSIRT owns triage, severity assessment, and the disclosure date for reports filed through the channels above. What that means in this repository, while a report is under embargo:

- Maintainers will not discuss the report in public issues, pull requests, or discussions.
- A fix will not be merged with a commit message, pull request description, or test name that reveals the vulnerability ahead of the coordinated date.
- No release will be tagged or announced in a way that advertises the issue before PSIRT publishes.
- The patched release and the advisory ship together on the coordinated date, and the `CHANGELOG.md` entry links to the advisory.
- Reporters who ask to remain anonymous are credited as "an anonymous reporter" rather than by name.

## NVIDIA Product Security

For all security-related concerns, please visit NVIDIA's Product Security portal at https://www.nvidia.com/en-us/security

## Supported Versions

NodeWright releases its components independently, each following [Semantic Versioning](https://semver.org/). Security fixes are released against the latest minor of each component. Critical fixes may be backported to the most recent prior release branch (`release/v{MAJOR.MINOR}.x`), at the maintainers' discretion; see [docs/operations/versioning.md](docs/operations/versioning.md).

| Component | Tag prefix | Supported |
|---|---|---|
| Operator | `operator/v` | Latest minor |
| Helm chart | `chart/v` | Latest minor |
| Agent | `agent/v` | Latest minor |
| CLI (`kubectl nodewright`) | `cli/v` | Latest minor |

The current version of each line is whatever the newest tag with that prefix says; this file deliberately does not restate it.

Older minors are not patched. If you are running one, upgrade to the latest release of that component. The full list is on the [releases page](https://github.com/NVIDIA/nodewright/releases).

Kubernetes version support is a separate policy: we CI-test and support the **latest four Kubernetes minor versions**. See [docs/operations/kubernetes-support.md](docs/operations/kubernetes-support.md) for the current window and what happens when it moves.

## Verifying Release Artifacts

Every released container image and the Helm chart is signed with [Sigstore cosign](https://docs.sigstore.dev/) in keyless mode, has a CycloneDX SBOM attached as an attestation, and carries [SLSA build provenance](https://slsa.dev/). Each signing job verifies its own output before finishing.

**Each artifact is signed by the workflow that builds it, gated on its own tag family, so the certificate identity differs per artifact.** A single identity pattern will not verify everything:

| Artifact | Signing workflow | Tag family |
|---|---|---|
| Operator image | `.github/workflows/operator-ci.yaml` | `refs/tags/operator/` |
| Agent image | `.github/workflows/agent-ci.yaml` | `refs/tags/agent/` |
| Helm chart | `.github/workflows/release.yml` | `refs/tags/chart/` |

Verify by digest, not by tag. A tag can be repointed between the moment you verify it and the moment you pull it; a digest cannot. This is also what the signing workflows themselves do.

```bash
REPO=ghcr.io/nvidia/nodewright/operator
ISSUER=https://token.actions.githubusercontent.com
# Operator image: pins the signing workflow AND the operator tag family.
# Swap both per the table above to verify the chart or the agent instead.
IDENTITY='^https://github\.com/NVIDIA/nodewright/\.github/workflows/operator-ci\.yaml@refs/tags/operator/.*$'

# Resolve the tag to an immutable digest once, then use it everywhere below
DIGEST=$(crane digest "$REPO:<tag>")
IMAGE="$REPO@$DIGEST"

# Signature (keyless, GitHub Actions OIDC identity)
cosign verify --certificate-oidc-issuer="$ISSUER" \
  --certificate-identity-regexp="$IDENTITY" "$IMAGE"

# SBOM attestation (CycloneDX)
cosign verify-attestation --type cyclonedx \
  --certificate-oidc-issuer="$ISSUER" \
  --certificate-identity-regexp="$IDENTITY" "$IMAGE"

# SLSA build provenance
cosign verify-attestation --type https://slsa.dev/provenance/v1 \
  --certificate-oidc-issuer="$ISSUER" \
  --certificate-identity-regexp="$IDENTITY" "$IMAGE"
```

These are the same three checks the signing workflow runs against its own output before it finishes. Pin the digest you verified in your Helm values or image reference, so the artifact you checked is the artifact that runs.

Without `crane`, use:

```bash
DIGEST=$(docker buildx imagetools inspect --format '{{.Manifest.Digest}}' "$REPO:<tag>")
```

Take the top-level manifest digest, which is what the signature and attestations are bound to. Do not substitute one of the per-platform digests listed under `Manifests:` in the plain `docker buildx imagetools inspect` output; those are children of the index and will not verify.

The identity regexp is deliberately narrow: it pins the signer to one workflow and one tag family in this repository, not merely to the NVIDIA organization. Loosening it to `refs/tags/` would let a signature produced by any other release path satisfy the check. Swap the workflow and tag family per the table above when verifying the chart or the agent.

Artifacts released before the Skyhook to NodeWright repository rename carry a `NVIDIA/skyhook` certificate identity; substitute that path when verifying older releases.

A verification failure on a published artifact is itself a security report. Route it through the channels above.
