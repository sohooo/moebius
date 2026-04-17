# Configuration

`møbius` supports three configuration sources for repository layout:

1. built-in defaults
2. optional repo-root `config.yaml`
3. optional `MOBIUS_CONFIG_YAML`

Targeted CLI overrides such as `--clusters-dir` apply on top.

## Default Layout

By default, cluster definitions live under `clusters/`.

Expected structure:
- `clusters/<cluster>/apps.yaml`
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
- apps file name
- field remapping for release entries
- required canonical fields
- primary and fallback override path patterns

The apps file must remain a top-level YAML list of release objects. `møbius` does not support nested release extraction, arbitrary YAML queries, or custom templating rules.

Example:

```yaml
layout:
  clusters_dir: clusters
  apps:
    file: apps.yaml
    fields:
      name: name
      namespace: namespace
      project: project
      repoURL: repoURL
      chart: chart
      targetRevision: targetRevision
  overrides:
    path: overrides/{project}/{name}.yaml
    fallback_path: overrides/{name}.yaml
```

## `MOBIUS_CONFIG_YAML`

For CI or containerized usage, the same schema can be passed through `MOBIUS_CONFIG_YAML`.

Example:

```yaml
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

## Field Remapping

`møbius` normalizes release entries to canonical fields internally.

Useful remapping cases:
- ArgoCD-style field names that differ from the defaults
- repos that use `releases.yaml` instead of `apps.yaml`
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
4. targeted CLI overrides such as `--clusters-dir`

## Practical Recommendations

- use defaults when the repo already matches the built-in layout
- use `config.yaml` for repo-owned conventions and local clarity
- use `MOBIUS_CONFIG_YAML` for CI/container setups that should not depend on a checked-in config file
- keep field remapping minimal and explicit
