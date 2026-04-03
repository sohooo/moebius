# møbius

`møbius` ("möbius") is a small CLI utility for GitOps workflows.

The name comes from *StarCraft*: the Moebius Foundation was a formerly legitimate research group focused on archaeology. It explored sites created by a race older than the protoss, including Research Site KL-2.

## Purpose

`møbius` is intended for GitLab merge request pipelines around Kubernetes cluster configuration managed with ArgoCD.

Its job is to:

- render the Helm charts that define a cluster from the merge-base with `master`
- render the same cluster configuration in the context of the merge request
- compare both rendered outputs chart by chart
- make the effective configuration change visible before the MR is merged

This gives reviewers a concrete view of what a merge request would actually change in the cluster instead of only showing raw YAML or values file edits.

## Cluster Layout

Cluster definitions live under `clusters/`.

- `clusters/<cluster>/apps.yaml` lists the Helm releases for that cluster
- `clusters/<cluster>/overrides/<chart-name>.yaml` optionally overrides chart values for the release whose `name` matches `<chart-name>`

Example:

- `clusters/kube-bravo/apps.yaml`
- `clusters/kube-bravo/overrides/hello-world.yaml`

The current demo repository also contains a sample chart under [charts/hello-world](/Users/sven/Code/lab/møbius/charts/hello-world).

## Implementation

The first implementation is a shell-based CLI at [bin/mobius](/Users/sven/Code/lab/møbius/bin/mobius).

It is intentionally small in scope and depends on a short list of external tools:

- `git`
- `helm`
- `yq`
- `diff`

## Usage

Render and compare changed clusters in the current branch:

```bash
bin/mobius diff
```

Render and compare one specific cluster:

```bash
bin/mobius diff --cluster kube-bravo
```

Persist rendered artifacts and per-chart diffs:

```bash
bin/mobius diff --cluster kube-bravo --output-dir .mobius-out
```

Available flags:

- `--clusters-dir PATH` default `clusters`
- `--base-ref REF` default `master`
- `--cluster NAME` force a single cluster
- `--all-clusters` process every cluster under `clusters/`
- `--output-dir PATH` keep rendered manifests and diff files
- `--context-lines N` set unified diff context

## How It Works

For each selected cluster, `møbius`:

1. reads `clusters/<cluster>/apps.yaml`
2. renders every release in that file with `helm template`
3. applies `clusters/<cluster>/overrides/<name>.yaml` if it exists
4. writes one manifest per release:
   - `current/<cluster>/<chart-name>.yaml`
   - `baseline/<cluster>/<chart-name>.yaml`
   - `diff/<cluster>/<chart-name>.diff`
5. prints unified diffs grouped by cluster and chart

The baseline is not the current tip of `master`. It is the git merge-base between `HEAD` and the configured base ref. This avoids unrelated drift from new commits on `master` while the MR is open.

## CI Expectations

`møbius` is designed for GitLab MR pipelines. The job environment should:

- fetch enough git history for `git merge-base`
- make the target branch ref, usually `master`, available locally
- provide `helm`, `yq`, `git`, and `diff`
- provide Helm registry authentication if any chart uses `oci://`

Example GitLab job:

```yaml
mobius-diff:
  stage: test
  image: alpine/helm:3.18.4
  before_script:
    - apk add --no-cache bash git diffutils yq
    - git fetch origin master
  script:
    - bin/mobius diff --output-dir .mobius-out
  artifacts:
    when: always
    paths:
      - .mobius-out/
```

## Scope

`møbius` should remain:

- small in footprint
- conservative in dependencies
- focused on rendering and diffing effective cluster configuration
