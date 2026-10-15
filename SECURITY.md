# Security policy

## Supported releases

Current source is rebuilt weekly with fresh stable OS packages and scanned;
this does not automatically publish a release. Only the newest non-prerelease
version is supported and scanned daily on both
Linux architectures. Fixes ship in new patch releases; published versions are
not silently replaced. Watch the `security` workflow and enable Actions failure
notifications. Report vulnerabilities privately through GitHub private vulnerability
reporting when enabled; never post credentials in public issues.

## Update cooldown

Renovate proposes updates weekly for Go, Docker, actions and scanners. Routine
updates (including security-alert PRs) require seven days since publication.
Missing timestamps hold updates for review. Automerge is disabled; major upgrades
also require dashboard approval. Review transitive changes before merging.

The cooldown reduces exposure to newly discovered problems; it does not prove
safety. Renovate cannot enforce age checks for every rollback, replacement or
lockfile-only operation. Packages installed from stable Alpine repositories by
apk are outside Renovate's cooldown and are scanned in the resulting image.

A maintainer can manually propose an urgent security fix before seven days,
recording the advisory, exact version, release date, rationale and scan/test
results. No automatic cooldown bypass is configured. Do not disable security
gates to merge an update.

The initial remediation explicitly updates x/net to v0.60.0 (published
2026-10-08), which fixes five reachable vulnerabilities reported by govulncheck;
this is a manual security fix, not an automatic age-policy exception.

## Gates and response

CI runs govulncheck and Trivy. Reachable Go vulnerabilities and High/Critical
image findings (including unfixed findings) block releases. Scanner or database
failures fail closed. Full JSON and SARIF reports are retained as Actions artifacts,
without requiring code-scanning write permissions on untrusted pull requests.

Triage exploitable Critical findings within one day and target fixes for High
findings within seven days. Track lower severities in the maintenance backlog.
The initial source scan reports GO-2026-5932 for the unmaintained
`golang.org/x/crypto/openpgp` package. govulncheck reports that the affected
package is not imported. Keep the advisory visible; no suppression is configured.

No vulnerability exceptions are currently configured. Future exceptions must
specify advisory, exact component scope, rationale, owner and expiry and include
an automated expiry check before use in releases.

## Release integrity

Binaries are scanned before upload. Each container architecture is built once and
pushed under a unique `candidate-*` tag to test and scan its immutable digest while
preserving SBOM/provenance attestations. Candidates are unapproved builds: consumers
must not use them. Release tags are assigned only after both architectures pass.
The index is signed using GitHub OIDC; archive/SBOM checksums have a Sigstore bundle.

Failed candidates may remain in GHCR. Configure retention without removing manifests
or attestations belonging to supported releases. If container publication succeeds
but the GitHub release fails, rerun only the failed publish job.

## Runtime compatibility

The CI image retains Git, grep, a shell and CA certificates. It supports explicit
non-root execution, a read-only root filesystem, dropped capabilities and
no-new-privileges. Mount writable checkout/output directories and /tmp. Root remains
the GitLab compatibility default until runner mount ownership is validated. Override
HOME/XDG_CACHE_HOME for persistent writable caches; mount internal CA certificates
as required.

A shell-free hardened image would break this CI contract. A future Docker Hardened
Image migration must preserve the tools, support both architectures, verify registry
access for fork builds and pass actual GitLab checkout/fetch/report jobs. The default
image has not been switched to an unvalidated DHI variant.
