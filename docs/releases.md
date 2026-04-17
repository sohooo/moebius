# Releases And Distribution

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
go install github.com/sohooo/moebius/cmd/mobius@v0.1.7
```

Container image:

```yaml
image: ghcr.io/sohooo/moebius:v0.1.7
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
