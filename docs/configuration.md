# Configuration

[Back to documentation index](index.md)

`møbius` resolves repository layout configuration in this precedence order:

1. built-in defaults
2. optional repo-root `config.yaml`
3. optional `MOBIUS_CONFIG_YAML`
4. optional `MOBIUS_APPS_FILES`
5. targeted CLI overrides such as `--clusters-dir` and `--apps-files`

## Default Layout

By default, cluster definitions live under `clusters/`.

Expected structure:
- `clusters/<cluster>/apps.yaml`
- `clusters/<cluster>/apps-dev.yaml` when a cluster has additional development/test releases
- `clusters/<cluster>/overrides/common.yaml`
- `clusters/<cluster>/overrides/<project>/<name>.yaml`
- `clusters/<cluster>/overrides/<name>.yaml`

Canonical release fields:
- `name`
- `namespace`
- `project`
- `repoURL`
- `chart`
- `targetRevision`

Remote charts are represented with:
- `repoURL`
- `chart`
- `targetRevision`

Local charts are represented with:
- `chart` as a local repo path

## `config.yaml`

`møbius` supports an optional repo-root [config.yaml](../config.yaml).

It can define:
- cluster root directory
- apps file names, in precedence order
- field remapping for release entries
- required canonical fields
- common, primary, and fallback override path patterns
- semantic diff ignore rules for non-actionable metadata churn

Each apps file must remain a top-level YAML list of release objects. `møbius` does not support nested release extraction, arbitrary YAML queries, or custom templating rules.

By default, `møbius` reads `apps.yaml` and `apps-dev.yaml` in that order. Missing files are skipped per cluster, and an existing empty secondary file simply contributes no releases. Earlier files have precedence: if the same release name appears in both files, the release from `apps.yaml` is used and the report highlights the duplicate definition as a warning.

Example:

```yaml
layout:
  clusters_dir: clusters
  apps:
    files:
      - apps.yaml
      - apps-dev.yaml
    fields:
      name: name
      namespace: namespace
      project: project
      repoURL: repoURL
      chart: chart
      targetRevision: targetRevision
  overrides:
    common_path: overrides/common.yaml
    path: overrides/{project}/{name}.yaml
    fallback_path: overrides/{name}.yaml
diff:
  ignore:
    defaults: true
```

## `MOBIUS_CONFIG_YAML`

For CI or containerized usage, the same schema can be passed through `MOBIUS_CONFIG_YAML`.

Example:

```yaml
layout:
  clusters_dir: environments
  apps:
    files:
      - releases.yaml
    fields:
      name: release_name
      namespace: target_namespace
      project: argocd_project
      repoURL: repo_url
      chart: chart_ref
      targetRevision: chart_target_revision
  overrides:
    common_path: values/common.yaml
    path: values/{project}/{name}.yaml
    fallback_path: values/{name}.yaml
```

## Override Values Order

In cluster repository mode, `møbius` renders each release with values files in this order:

| Order | Default path | Purpose |
| ---: | --- | --- |
| 1 | `clusters/<cluster>/overrides/common.yaml` | Optional cluster-wide values shared by multiple releases. |
| 2 | `clusters/<cluster>/overrides/<project>/<name>.yaml` | Primary release-specific values. |
| 3 | `clusters/<cluster>/overrides/<name>.yaml` | Fallback release-specific values, used only when the primary file is absent. |

Helm merges values in listed order, so release-specific values override `common.yaml`. Missing `common.yaml` is normal and does not produce a warning.

## Field Remapping

`møbius` normalizes release entries to canonical fields internally.

Useful remapping cases:
- ArgoCD-style field names that differ from the defaults
- repos that use `releases.yaml` instead of `apps.yaml`
- test clusters that add `apps-dev.yaml` alongside `apps.yaml`
- alternative override file naming schemes

Example field remapping:

```yaml
layout:
  apps:
    fields:
      name: release_name
      namespace: target_namespace
      project: argocd_project
      repoURL: repo_url
      chart: chart_ref
      targetRevision: chart_target_revision
```

## Precedence

Configuration precedence is:

1. built-in defaults
2. optional repo-root `config.yaml`
3. optional `MOBIUS_CONFIG_YAML`
4. optional `MOBIUS_APPS_FILES`
5. targeted CLI overrides such as `--clusters-dir` and `--apps-files`

`MOBIUS_APPS_FILES` and `--apps-files` accept comma-separated files, for example `apps.yaml,apps-dev.yaml`.

## Diff Ignore Rules

`møbius` suppresses common Helm metadata churn by default:

- `metadata.labels.app.kubernetes.io/version`
- `metadata.labels.helm.sh/chart`
- `*.metadata.annotations.checksum/*` at common resource and pod-template metadata locations

This keeps chart version labels and render-time checksum annotations from creating noisy MR report entries when no actionable resource fields changed.

Disable the built-ins:

```yaml
diff:
  ignore:
    defaults: false
```

Add repo-specific metadata ignore rules:

```yaml
diff:
  ignore:
    defaults: true
    metadata:
      - locations:
          - metadata
          - spec.template.metadata
          - spec.jobTemplate.spec.template.metadata
        labels:
          - app.kubernetes.io/version
          - helm.sh/chart
        annotations:
          - checksum/*
          - rollme
```

Rules are intentionally limited to labels and annotations. `locations` are exact semantic paths to metadata objects; `labels` and `annotations` are key patterns where `*` matches within the full key, for example `checksum/*` matches `checksum/config` and `checksum/secret`.

If every semantic change in a resource is ignored, the resource is treated as unchanged and omitted from the report. If a resource has both ignored and actionable changes, only the actionable changes are shown.

### CI Overrides

Use `MOBIUS_CONFIG_YAML` to change ignore behavior in a GitLab pipeline without editing the checked-in `config.yaml`:

```yaml
variables:
  MOBIUS_CONFIG_YAML: |
    diff:
      ignore:
        defaults: true
        metadata:
          - locations:
              - metadata
              - spec.template.metadata
              - spec.jobTemplate.spec.template.metadata
            labels:
              - app.example.com/build-id
            annotations:
              - checksum/*
              - rollme
```

`MOBIUS_CONFIG_YAML` is applied as a config overlay, not as an append-only list operation. If the repo already defines `diff.ignore.metadata` and the pipeline should add one more rule, put the full desired `metadata` list in `MOBIUS_CONFIG_YAML`.

## Practical Recommendations

- use defaults when the repo already matches the built-in layout
- use `config.yaml` for repo-owned conventions and local clarity
- use `MOBIUS_CONFIG_YAML` for CI/container setups that should not depend on a checked-in config file
- keep field remapping minimal and explicit
- keep diff ignore rules narrow; prefer labels and annotations that are known render noise
