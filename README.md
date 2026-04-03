# møbius

`møbius` ("möbius") is a small CLI utility for GitOps workflows.

The name comes from *StarCraft*: the Moebius Foundation was a formerly legitimate research group focused on archaeology. It explored sites created by a race older than the protoss, including Research Site KL-2.

## Purpose

`møbius` is intended for GitLab merge request pipelines around Kubernetes cluster configuration managed with ArgoCD.

Its job is to:

- render the Helm charts that define a cluster from the merge-base with `master`
- render the same cluster configuration in the context of the merge request
- compare both rendered outputs chart by chart and resource by resource
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

`møbius` is now a native Go CLI. The compiled binary is named exactly `møbius` and is built to `bin/møbius` for local use.

It is self-contained at runtime and does not depend on external `git`, `helm`, `yq`, `diff`, or `delta` executables. Instead, it uses Go libraries for:

- Git repository access and merge-base resolution
- Helm chart loading and rendering
- YAML parsing and resource splitting
- raw unified diffs and semantic YAML diffs

`bin/møbius` is a generated build artifact and is ignored in Git.

## Usage

Build the native binary:

```bash
make build
```

Run the standard local verification pass:

```bash
make verify
```

Render and compare changed clusters in the current branch:

```bash
./bin/møbius diff
```

Render and compare one specific cluster:

```bash
./bin/møbius diff --cluster kube-bravo
```

Post or update the sticky merge request comment from a GitLab MR pipeline:

```bash
./bin/møbius comment
```

Render markdown locally for copy and paste:

```bash
make diff-markdown
```

Persist rendered artifacts and diff outputs:

```bash
./bin/møbius diff --cluster kube-bravo --output-dir .mobius-out
```

Available flags:

- `--clusters-dir PATH` default `clusters`
- `--base-ref REF` default `master`
- `--cluster NAME` force a single cluster
- `--all-clusters` process every cluster under `clusters/`
- `--output-dir PATH` keep rendered manifests and diff files
- `--context-lines N` set unified diff context
- `--diff-mode raw|semantic|both` choose output mode
- `--output-format plain|markdown` choose terminal output or markdown-ready output
- `comment` supports `--project-id`, `--mr-iid`, and `--gitlab-base-url` overrides for manual testing

Markdown mode is intended for copy and paste into merge requests or documentation. It uses markdown headings, fenced `diff` blocks, and markdown summary tables.

The `comment` subcommand always uses markdown output internally and posts it to the merge request as a single sticky bot note.
If the sticky note body is already up to date, `møbius` leaves it unchanged instead of issuing another update.

## How It Works

For each selected cluster, `møbius`:

1. reads `clusters/<cluster>/apps.yaml`
2. resolves the merge-base with the configured base ref using native Git handling
3. renders every release in that file with the Helm Go SDK
4. applies `clusters/<cluster>/overrides/<name>.yaml` if it exists
5. writes one manifest per release:
   - `current/<cluster>/<chart-name>/rendered.yaml`
   - `current/<cluster>/<chart-name>/resources/<kind>--<namespace-or-cluster>--<name>.yaml`
   - `baseline/<cluster>/<chart-name>/resources/<kind>--<namespace-or-cluster>--<name>.yaml`
   - `diff/<cluster>/<chart-name>/<resource-key>.diff`
   - `diff/<cluster>/<chart-name>/<resource-key>.semantic.txt`
6. prints diffs grouped by cluster, chart, and resource

Console output includes:

- the cluster name
- the chart name and release namespace
- the Kubernetes resource identity as `Kind/name`
- semantic YAML context back to the changed path root in `semantic` mode

The baseline is not the current tip of `master`. It is the git merge-base between `HEAD` and the configured base ref. This avoids unrelated drift from new commits on `master` while the MR is open.

## CI Expectations

`møbius` is designed for GitLab MR pipelines. The job environment should:

- fetch enough git history for `git merge-base`
- make the target branch ref, usually `master`, available locally
- provide the repository checkout
- provide `CI_PROJECT_ID`, `CI_MERGE_REQUEST_IID`, and `CI_JOB_TOKEN`
- provide either `CI_API_V4_URL` or `CI_SERVER_URL`
- provide network and credentials only if OCI chart access requires them

Example GitLab job:

```yaml
mobius-diff:
  stage: test
  image: golang:1.25
  before_script:
    - make build
  script:
    - ./bin/møbius comment --output-dir .mobius-out
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
