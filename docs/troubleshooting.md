# Troubleshooting

This page focuses on concrete symptom → cause → fix guidance for `møbius`.

Start with artifacts when available:
- `.mobius-out/comment-preflight.json`
- `.mobius-out/index.md`
- `.mobius-out/summary.json`
- `.mobius-out/errors/`
- `.mobius-out/warnings/`

## `could not auto-detect a base ref`

What it means:
- `møbius` could not resolve any default base ref automatically

Why it happens:
- `refs/remotes/origin/HEAD` is not present locally
- neither `main` nor `master` exists locally
- GitLab MR pipelines often run in detached checkout mode
- the target branch is not fetched automatically as a local ref

How to verify:

```bash
git branch --list
git branch -r
```

Fix:

```yaml
variables:
  GIT_DEPTH: "0"
script:
  - git fetch origin "${CI_MERGE_REQUEST_TARGET_BRANCH_NAME}:${CI_MERGE_REQUEST_TARGET_BRANCH_NAME}"
  - |
    møbius comment \
      --base-ref "${CI_MERGE_REQUEST_TARGET_BRANCH_NAME}" \
      --output-dir .mobius-out
```

If you do not set `--base-ref`, `møbius` now auto-detects the base ref in this order:
- `origin/HEAD`
- `main`
- `master`

If you set `--base-ref master` explicitly, you may still see the older error:

```text
could not resolve git revision "master"
```

## `401 Unauthorized` on MR publishing

What it means:
- the resolved GitLab token is missing, invalid, or not accepted for the GitLab MR API

Most common cause:
- relying on `CI_JOB_TOKEN` for MR description updates or note creation

How to verify:
- open `.mobius-out/comment-preflight.json`
- check:
  - `token_source`
  - `token_kind`
  - `messages`

Fix:
- use `GITLAB_API_TOKEN` or `--gitlab-token`
- use a project, group, or bot token with API scope
- `GITLAB_TOKEN` and `GITLAB_PRIVATE_TOKEN` are accepted aliases, but prefer `GITLAB_API_TOKEN` for new pipelines

Example:

```yaml
variables:
  GITLAB_API_TOKEN: "${MOBIUS_GITLAB_API_TOKEN}"
```

## Token can read but cannot publish the MR report

What it means:
- the token can reach the merge request API
- it does not have permission to update the MR description, or notes when `--publish-target note` is used

Typical case:
- `CI_JOB_TOKEN` works for read/list, but not for MR description updates or note creation

How to verify:
- inspect `.mobius-out/comment-preflight.json`
- look for a message like:
  - token can read the merge request but cannot update its description

Fix:
- replace `CI_JOB_TOKEN` with `GITLAB_API_TOKEN`
- use a write-capable GitLab API token

`GITLAB_TOKEN` and `GITLAB_PRIVATE_TOKEN` remain supported aliases. `CI_JOB_TOKEN` is useful context inside GitLab jobs, but it is not a publishing token for the MR report.

## Invalid rendered YAML

Example:
- duplicate mapping keys
- malformed rendered manifest output

What it means:
- one rendered chart output is invalid before `møbius` can split it into resources

How to verify:
- open `.mobius-out/errors/<state>--<cluster>--<release>.txt`
- inspect the referenced `rendered.yaml`

Example local verification:

```bash
helm dependency update
helm template otel-stack . > rendered.yaml
```

Fix options:
- fix or patch the chart
- use `--render-error-mode warn-skip-release` if you want the rest of the diff to continue

## Duplicate YAML keys

What it means:
- one rendered YAML mapping contains the same key more than once

Recommended default:
- treat this as invalid YAML and fix the chart

Fallback option:

```bash
møbius comment --duplicate-key-mode warn-last-wins
```

Behavior in that mode:
- `møbius` keeps the last duplicate value
- it records a warning
- the report is explicitly marked as non-strict

Verify via:
- `.mobius-out/warnings/`
- `.mobius-out/comment-preflight.json`
- stdout / MR report warnings

## `.mobius-out` is missing

What it means:
- either `--output-dir` was not actually passed
- or the command wrote to a temporary directory because the flag was not parsed

Most common cause:
- broken multiline shell formatting in GitLab YAML

Use:

```yaml
script:
  - |
    møbius comment \
      --base-ref "${CI_MERGE_REQUEST_TARGET_BRANCH_NAME}" \
      --output-dir .mobius-out
```

Do not rely on a backslash unless it is the last character before the newline.

## `No affected clusters.`

What it means:
- no clusters changed between `HEAD` and the resolved base ref

What `møbius` now shows:
- effective clusters directory
- resolved base ref
- a hint to use `--all-clusters` or `mobius clusters`

How to verify:

```bash
mobius clusters
mobius diff --all-clusters
```

## Cluster is missing when using multiple apps files

What it means:
- the selected cluster was not discoverable from the configured apps files
- or the expected apps file exists only in a file name that `møbius` was not configured to load

How to verify:

```bash
mobius clusters --apps-files apps.yaml,apps-dev.yaml
mobius doctor --apps-files apps.yaml,apps-dev.yaml
```

Fix:
- configure all apps files in [configuration.md](configuration.md), `MOBIUS_APPS_FILES`, or `--apps-files`
- keep the list in precedence order, for example `apps.yaml,apps-dev.yaml`
- remember that missing secondary files are skipped per cluster, but at least one configured apps file must exist for the cluster to be loadable

## Resource is unvalidated or schema coverage is missing

What it means:
- the resource passed structural parsing, but no matching schema was available
- the report may show this as `unvalidated`, a validation gap, or an Attention Required item for critical/high resources

Why it happens:
- the resource kind is a CRD that is not rendered in the current manifests
- the embedded schema bundle does not include that CRD family or version
- the CRD schema exists, but the resource uses a different API version

Fix:
- inspect the report's Validation Gaps and Attention Required sections; see [report.md](report.md)
- check bundled schema versions in [schema-bundles.md](schema-bundles.md)
- render the CRD together with the resources under review, or update the embedded schema bundle if the CRD is maintained as a shared platform schema

## Helm dependency or chart repository authentication fails

What it means:
- Helm could not render one release before `møbius` could compare resources

How to verify:
- open `.mobius-out/errors/<state>--<cluster>--<release>.txt`
- check whether the error mentions a missing dependency, repository authentication, OCI pull failure, or chart version lookup

Fix:
- make sure the GitLab job has the same Helm repository or registry credentials as the normal render path
- run `helm dependency update` for local charts that require vendored dependencies
- set `targetRevision` for remote and OCI charts
- use `--render-error-mode warn-skip-release` only when reviewers should still see the rest of the report

## GitLab CI cannot pull the GHCR image

What it means:
- the runner could not pull `ghcr.io/sohooo/moebius:vX.Y.Z`

How to verify:
- confirm the tag exists in GHCR for the same release version
- check whether the GitLab runner requires registry authentication for GHCR
- see [releases.md](releases.md) if the git tag exists but the image is missing

Fix:
- use an explicit semver image tag, not `latest`
- add GitLab CI registry credentials for GHCR if your runner cannot pull public images anonymously
- if the image was never published, inspect the GitHub Actions `container` job for the release tag

## Report falls back to summary mode

What it means:
- the full GitLab MR report exceeded the configured comment size limit
- `møbius` published a compact summary instead of the full detail

How to verify:
- inspect `.mobius-out/summary.json`
- open `.mobius-out/index.md` and per-resource artifacts for the full detail

Fix:
- keep `.mobius-out/` as a job artifact
- use `--comment-mode summary+artifacts` intentionally for large repos
- raise `--max-comment-bytes` only if your GitLab instance accepts larger MR descriptions or notes

## Tag exists but GitHub Release or GHCR image is missing

What it means:
- the git tag was pushed
- the release workflow failed before publishing artifacts or the container

How to verify:
- open the GitHub Actions run for the tag
- check:
  - `release` job
  - `container` job

Typical causes seen so far:
- stale tests failing in the release workflow
- multi-arch Docker packaging issues

Fix:
- fix the failing workflow step
- cut a new semver tag instead of mutating the old one

## `go install` fails right after a new tag

What it means:
- the new tag exists
- Go proxy or checksum infrastructure has not caught up yet

Temporary workaround:

```bash
GOPROXY=direct GOSUMDB=off go install github.com/sohooo/moebius/cmd/mobius@vX.Y.Z
```

How to verify installation:

```bash
mobius version
```

This should only be necessary shortly after a new public tag.
