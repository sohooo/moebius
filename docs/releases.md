# Releases And Distribution

[Back to documentation index](index.md)

`møbius` is distributed in three ways:

- Go install:
  - `go install github.com/sohooo/moebius/cmd/mobius@vX.Y.Z`
- GHCR image:
  - `ghcr.io/sohooo/moebius:vX.Y.Z`
- GitHub Releases:
  - CLI archives and `checksums.txt`

## Install Paths

Local CLI install:

```bash
go install github.com/sohooo/moebius/cmd/mobius@latest
```

Pinned install:

```bash
go install github.com/sohooo/moebius/cmd/mobius@vX.Y.Z
```

Container image:

```yaml
image: ghcr.io/sohooo/moebius:vX.Y.Z
```

Notes:
- `go install` produces `mobius`
- the container image ships both `mobius` and `møbius`
- the container entrypoint uses `møbius`

## Release Policy

- use semver tags
- stay on `v0.x.y` while the CLI/output surface is still evolving
- do not rely on `latest` for GHCR during `v0`
- use explicit image tags in CI

## Maintainer Checklist

For each release:

1. run `make schema-verify`
2. run `make verify`
3. create a semver git tag on `master`
4. push `master`
5. push the tag
6. confirm:
   - GitHub Release exists
   - archives and `checksums.txt` are attached
   - GHCR image `ghcr.io/sohooo/moebius:vX.Y.Z` exists

## Release Workflow Outputs

The tag-driven release workflow should produce:
- GitHub Release notes
- Linux and Darwin archives for `amd64` and `arm64`
- `checksums.txt`
- GHCR image for the same tag

If the tag exists but the Release page or GHCR image is missing, see [troubleshooting.md](troubleshooting.md).

## Security and provenance

Run `make vulncheck` before tagging. Release jobs scan every binary and both image
architectures. Assets include CycloneDX SBOMs and signed checksums; containers
include BuildKit SBOM/provenance attestations and keyless signatures.

Verify an image by digest (replace VERSION and DIGEST):

```bash
cosign verify \
  --certificate-identity 'https://github.com/sohooo/moebius/.github/workflows/release.yml@refs/tags/VERSION' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  ghcr.io/sohooo/moebius@sha256:DIGEST
```

Verify downloaded assets:

```bash
cosign verify-blob --bundle checksums.sigstore.json \
  --certificate-identity 'https://github.com/sohooo/moebius/.github/workflows/release.yml@refs/tags/VERSION' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com checksums.txt
sha256sum --check checksums.txt
```

Enable the Renovate GitHub App for this repository to activate `renovate.json`.
Require the `ci / verify` check in branch protection and enable Actions failure
notifications for `security`. Scheduled monitoring alerts on new/changed findings
and weekly for unresolved findings; operational errors always fail. These settings
are separate from committed files. Confirm Renovate activation through its
onboarding PR or Dependency Dashboard; configuration alone does not run the bot.

Weekly update PRs use a seven-day cooldown and require review. Merge and release
approved fixes promptly: scanning does not patch published artifacts. See
[the security policy](../SECURITY.md) for supported versions and emergency fixes.


### Recover a missing push-triggered release

If GitHub does not start a tag-push workflow, dispatch the same gated pipeline
against the existing tag; do not move or recreate a published tag:

```bash
gh workflow run release.yml --ref master -f tag=vX.Y.Z
```

The workflow validates the version and checks out `refs/tags/vX.Y.Z` in build
jobs. Build metadata records that checkout's commit. The manual run's signing
identity ends in `release.yml@refs/heads/master`; use that exact identity instead
of `release.yml@refs/tags/VERSION` in the verification commands above. The
release tag and image revision still identify the source being released.
