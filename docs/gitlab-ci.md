# GitLab CI Guide

`møbius` is designed to run in a GitLab merge request pipeline, usually from the cluster configuration repository itself.

Its main CI job is:
- render the effective cluster state at the merge-base
- render the effective cluster state at the MR commit
- compare both
- publish the result as a sticky merge request note or as job output

## Canonical MR Pipeline

Recommended pipeline job:

```yaml
mobius-diff:
  stage: test
  image: ghcr.io/sohooo/moebius:v0.1.8
  tags:
    - k8s
  variables:
    GIT_DEPTH: "0"
    GITLAB_TOKEN: "${MOBIUS_GITLAB_TOKEN}"
  script:
    - git fetch origin "${CI_MERGE_REQUEST_TARGET_BRANCH_NAME}:${CI_MERGE_REQUEST_TARGET_BRANCH_NAME}"
    - |
      møbius comment \
        --base-ref "${CI_MERGE_REQUEST_TARGET_BRANCH_NAME}" \
        --output-dir .mobius-out
  artifacts:
    when: always
    paths:
      - .mobius-out/
```

Why these settings matter:
- `GIT_DEPTH: "0"` gives `møbius` enough history for merge-base calculation
- the explicit `git fetch` makes the target branch ref available locally in detached MR jobs
- `GITLAB_TOKEN` provides note-writing permission for `møbius comment`
- `.mobius-out/` is the canonical debug surface

## Required Environment

For `møbius comment`, the job needs:
- `CI_PROJECT_ID`
- `CI_MERGE_REQUEST_IID`
- `CI_API_V4_URL` or `CI_SERVER_URL`
- `GITLAB_TOKEN`, `GITLAB_PRIVATE_TOKEN`, `GITLAB_API_TOKEN`, or `--gitlab-token`

Recommended token model:
- use a project, group, or bot token with API scope
- treat `CI_JOB_TOKEN` as fallback only

`CI_JOB_TOKEN` often cannot create or update merge request notes, even when it can read the MR.

## Comment Preflight

Before rendering and posting, `møbius comment` validates:
- project ID
- merge request IID
- GitLab API base URL
- resolved token source and kind
- ability to reach the MR notes API
- ability to create or update MR notes

If preflight fails, `møbius comment`:
- writes any available artifacts
- prints a concise failure summary
- falls back to printing the diff report to stdout
- exits non-zero

## Artifacts

When `--output-dir .mobius-out` is used, `møbius` writes:

- `.mobius-out/index.md`
- `.mobius-out/summary.json`
- `.mobius-out/comment-preflight.json`
- `.mobius-out/current/...`
- `.mobius-out/baseline/...`
- `.mobius-out/diff/...`
- `.mobius-out/errors/<state>--<cluster>--<release>.txt`
- `.mobius-out/warnings/<state>--<cluster>--<release>.txt`

Use these artifacts first when debugging CI failures.

For a fast local preflight before touching CI, run:

```bash
mobius doctor
```

## Common Variants

If you want CI to keep reporting the rest when one release renders invalid YAML:

```yaml
script:
  - git fetch origin "${CI_MERGE_REQUEST_TARGET_BRANCH_NAME}:${CI_MERGE_REQUEST_TARGET_BRANCH_NAME}"
  - |
    møbius comment \
      --base-ref "${CI_MERGE_REQUEST_TARGET_BRANCH_NAME}" \
      --render-error-mode warn-skip-release \
      --output-dir .mobius-out
```

If a third-party chart emits duplicate YAML keys and you need permissive last-key-wins parsing:

```yaml
script:
  - git fetch origin "${CI_MERGE_REQUEST_TARGET_BRANCH_NAME}:${CI_MERGE_REQUEST_TARGET_BRANCH_NAME}"
  - |
    møbius comment \
      --base-ref "${CI_MERGE_REQUEST_TARGET_BRANCH_NAME}" \
      --duplicate-key-mode warn-last-wins \
      --output-dir .mobius-out
```

If you only want job output and no MR note:

```yaml
script:
  - git fetch origin "${CI_MERGE_REQUEST_TARGET_BRANCH_NAME}:${CI_MERGE_REQUEST_TARGET_BRANCH_NAME}"
  - |
    møbius diff \
      --base-ref "${CI_MERGE_REQUEST_TARGET_BRANCH_NAME}" \
      --output-format markdown \
      --output-dir .mobius-out
```

## Custom Layouts

If the repo does not use the default layout, pass `MOBIUS_CONFIG_YAML`:

```yaml
variables:
  MOBIUS_CONFIG_YAML: |
    layout:
      clusters_dir: environments
      apps:
        file: releases.yaml
        fields:
          name: release_name
          namespace: target_namespace
          project: argocd_project
          repoURL: repo_url
          chart: chart_ref
          targetRevision: chart_target_revision
      overrides:
        path: values/{project}/{name}.yaml
        fallback_path: values/{name}.yaml
```

More detail is in [configuration.md](configuration.md).

## Failure Triage

Start here:
1. open `.mobius-out/comment-preflight.json`
2. open `.mobius-out/index.md`
3. inspect `.mobius-out/errors/` and `.mobius-out/warnings/`
4. if the failure is GitLab-specific, see [troubleshooting.md](troubleshooting.md)
