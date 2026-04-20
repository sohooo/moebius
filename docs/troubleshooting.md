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

## `401 Unauthorized` on MR notes

What it means:
- the resolved GitLab token is missing, invalid, or not accepted for the notes API

Most common cause:
- relying on `CI_JOB_TOKEN` for note creation

How to verify:
- open `.mobius-out/comment-preflight.json`
- check:
  - `token_source`
  - `token_kind`
  - `messages`

Fix:
- use `GITLAB_TOKEN` or `--gitlab-token`
- use a project, group, or bot token with API scope

Example:

```yaml
variables:
  GITLAB_TOKEN: "${MOBIUS_GITLAB_TOKEN}"
```

## Token can read but cannot create MR notes

What it means:
- the token can reach the merge request notes API
- it does not have permission to create or update notes

Typical case:
- `CI_JOB_TOKEN` works for read/list, but not for note creation

How to verify:
- inspect `.mobius-out/comment-preflight.json`
- look for a message like:
  - token can read the merge request but cannot create MR notes

Fix:
- replace `CI_JOB_TOKEN` with `GITLAB_TOKEN`
- use a write-capable GitLab API token

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
- stdout / MR note warnings

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
