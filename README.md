# møbius

`møbius` ("möbius") is a small Go CLI for GitOps repositories that manage Kubernetes clusters with ArgoCD.

Its job is to render the effective Helm-based cluster configuration for:
- the merge-base with the target branch
- the current merge request state

Then it compares both rendered results chart by chart and resource by resource, so reviewers can see what the merge request would actually change in the cluster.

> The name comes from *StarCraft*: the Moebius Foundation was a formerly legitimate research group focused on archaeology. It explored sites created by a race older than the protoss, including Research Site KL-2.

## CI Usage

`møbius` is designed to run as a separate tool in a GitLab merge request pipeline, typically from the cluster configuration repository. A common production setup is to build and publish the `møbius` image once, then run that image on a Kubernetes GitLab runner.

### What `møbius comment` Does

When a GitLab MR pipeline runs `møbius comment`, it:

1. detects the current merge request from the GitLab CI environment
2. renders the effective cluster state at the merge-base and at the MR commit
3. builds a markdown diff report like the sample in [docs/sample-report.md](docs/sample-report.md)
4. finds the existing `møbius` MR note, if one already exists
5. creates that note if it does not exist yet
6. updates the same note on later pipeline runs instead of creating duplicates
7. leaves the note unchanged if the rendered report body is already up to date

The posted note contains:

- a per-cluster summary table
- chart and namespace headers
- resource-level diff sections
- semantic diff snippets that highlight the effective Kubernetes changes

If there are no effective changes, `møbius comment` updates the sticky note to a short no-change message instead of deleting it.

### Why This Is Useful

Using `møbius comment` in the MR pipeline makes the rendered diff part of the merge request discussion itself.

That gives reviewers:

- a stable, visible diff directly on the MR instead of only in job logs
- a report they can quote, reference, and discuss in review threads
- an updated view on every pipeline run without accumulating multiple bot comments
- a clearer picture of the effective cluster change than raw values-file edits alone

The job environment should:

- fetch enough git history for merge-base calculation
- make the target branch ref, usually `master`, available locally
- provide the repository checkout
- provide `CI_PROJECT_ID`, `CI_MERGE_REQUEST_IID`, and `CI_JOB_TOKEN`
- provide either `CI_API_V4_URL` or `CI_SERVER_URL`
- provide network and credentials only if OCI chart access requires them

The repository in which the pipeline runs should include [config.yaml](config.yaml), the cluster definitions, and any referenced local charts.

Example GitLab job:

```yaml
mobius-diff:
  stage: test
  image: registry.example.com/platform/møbius:latest
  tags:
    - k8s
  script:
    - møbius comment --output-dir .mobius-out
  artifacts:
    when: always
    paths:
      - .mobius-out/
```

If you prefer to keep the diff only in job output, use `møbius diff`. If you want the report directly on the merge request, use `møbius comment`.

## Sample Report

A sample markdown report is available in [docs/sample-report.md](docs/sample-report.md).

`møbius` can render markdown output for copy and paste, or post the same style of report directly into a GitLab merge request as a sticky bot comment.

## How It Works

For each selected cluster, `møbius`:

1. reads [config.yaml](config.yaml) from the repository root
2. resolves the merge-base with the configured base ref
3. loads the configured apps file for each cluster
4. renders each release with the Helm Go SDK
5. applies override values from the configured override path
6. splits the rendered output into individual Kubernetes resources
7. compares baseline and current resources semantically and as raw text
8. renders the result as terminal output, markdown, or a GitLab MR note

```mermaid
flowchart TD
    A["Load config.yaml"] --> B["Select clusters"]
    B --> C["Resolve merge-base"]
    C --> D["Build baseline workspace"]
    C --> E["Use current worktree"]
    D --> F["Load apps file"]
    E --> F
    F --> G["Render Helm releases"]
    G --> H["Split into resources"]
    H --> I["Compare baseline vs current"]
    I --> J["Terminal diff"]
    I --> K["Markdown report"]
    I --> L["GitLab MR comment"]
```

Artifacts are written per chart and per resource:

- `current/<cluster>/<chart>/rendered.yaml`
- `current/<cluster>/<chart>/resources/<kind>--<namespace-or-cluster>--<name>.yaml`
- `baseline/<cluster>/<chart>/resources/<kind>--<namespace-or-cluster>--<name>.yaml`
- `diff/<cluster>/<chart>/<resource-key>.diff`
- `diff/<cluster>/<chart>/<resource-key>.semantic.txt`

The baseline is the git merge-base between `HEAD` and the configured base ref, not the current tip of the target branch.

## Quickstart

Build the binary:

```bash
make build
```

Run the standard verification pass:

```bash
make verify
```

Render the diff for one cluster:

```bash
./bin/møbius diff --cluster kube-bravo
```

Render markdown output that is ready to paste into a merge request:

```bash
./bin/møbius diff --cluster kube-bravo --output-format markdown
```

Post or update the sticky merge request comment from a GitLab MR pipeline:

```bash
./bin/møbius comment
```

Persist rendered artifacts and diffs:

```bash
./bin/møbius diff --cluster kube-bravo --output-dir .mobius-out
```

Build the container image:

```bash
docker build -t mobius:local .
```

## Cluster Layout

By default, cluster definitions live under `clusters/`.

- `clusters/<cluster>/apps.yaml` lists the Helm releases for that cluster
- each release entry can include fields such as `name`, `namespace`, `project`, `chart`, and `version`
- `clusters/<cluster>/overrides/<project>/<chart>.yaml` is the default primary override path
- `clusters/<cluster>/overrides/<chart>.yaml` is the default fallback override path

Example:

- `clusters/kube-bravo/apps.yaml`
- `clusters/kube-bravo/overrides/test/hello-world.yaml`

The demo repository also contains a sample chart under [charts/hello-world](charts/hello-world).

## Repository Config

`møbius` requires a root-level [config.yaml](config.yaml).

It defines:

- the cluster root directory
- the apps file name inside each cluster
- the field names used inside each release entry
- which canonical fields are required
- the primary and fallback override path patterns

The apps file is expected to be a top-level YAML list of release objects. `møbius` does not support nested release extraction, arbitrary YAML queries, or custom templating rules in `config.yaml`.

Example field remapping:

```yaml
layout:
  apps:
    fields:
      name: release_name
      namespace: target_namespace
      project: argocd_project
      chart: chart_ref
      version: chart_version
```

`--clusters-dir` is available as an explicit override for `layout.clusters_dir`.

## CLI Reference

| Flag | Meaning | Default / Notes |
| --- | --- | --- |
| `--clusters-dir PATH` | Override the cluster root directory from `config.yaml` | No override |
| `--base-ref REF` | Base ref used for merge-base comparison | `master` |
| `--cluster NAME` | Render and compare one specific cluster | Optional |
| `--all-clusters` | Render and compare all clusters | Optional |
| `--output-dir PATH` | Keep rendered manifests and diff files | Temporary dir otherwise |
| `--context-lines N` | Unified diff context lines | `3` |
| `--diff-mode raw\|semantic\|both` | Select raw diff, semantic diff, or both | `semantic` |
| `--output-format plain\|markdown` | Select terminal output format | `plain` |
| `--project-id` | Override GitLab project ID for `comment` mode | `CI_PROJECT_ID` |
| `--mr-iid` | Override GitLab MR IID for `comment` mode | `CI_MERGE_REQUEST_IID` |
| `--gitlab-base-url` | Override GitLab API base URL for `comment` mode | `CI_API_V4_URL` or `CI_SERVER_URL` |

The `comment` subcommand always renders markdown internally and updates a single sticky MR note. If the note body is already current, it leaves the note unchanged.

## Implementation Notes

`møbius` is a native Go CLI built to `bin/møbius`.

It is self-contained at runtime and uses Go libraries for:

- Git repository access and merge-base resolution
- Helm chart loading and rendering
- YAML parsing and resource splitting
- raw unified diffs and semantic YAML diffs

`bin/møbius` is a generated build artifact and is ignored in Git.

The repository also includes a [Dockerfile](Dockerfile) for building a small runtime image that contains only the compiled `møbius` binary and CA certificates.
