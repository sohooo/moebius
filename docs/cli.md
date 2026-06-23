# CLI Reference

This page summarizes the commands and flags most users need. Run `mobius --help` for the exact help text in the installed binary.

## Commands

| Command | Use |
| --- | --- |
| `mobius version` | Print build metadata for the installed binary. |
| `mobius clusters` | List discoverable clusters for the current repo and base ref. |
| `mobius doctor` | Run local preflight checks; with GitLab context, also checks MR publishing readiness. |
| `mobius diff` | Render, compare, validate, and print the report to stdout. |
| `mobius comment` | Render, compare, validate, and publish a managed GitLab MR report. |

## Common Flags

| Flag | Commands | Purpose |
| --- | --- | --- |
| `--base-ref <ref>` | `clusters`, `doctor`, `diff`, `comment` | Base ref used to calculate the merge-base. In GitLab MR pipelines, use the target branch name. |
| `--cluster <name>` | `clusters`, `doctor`, `diff`, `comment` | Limit work to one cluster. |
| `--all-clusters` | `diff`, `comment` | Render all current clusters instead of only changed clusters. |
| `--chart-path <path>` | `diff`, `comment` | Run chart repository mode against one local Helm chart. Defaults to `.` when chart mode is auto-detected. |
| `--values-files <list>` | `diff`, `comment` | Chart mode values files relative to `--chart-path`, merged in order. |
| `--release-name <name>` | `diff`, `comment` | Chart mode Helm release name. Defaults to `Chart.yaml` `name`. |
| `--namespace <name>` | `diff`, `comment` | Chart mode Helm release namespace. Defaults to `default`. |
| `--clusters-dir <path>` | `clusters`, `doctor`, `diff`, `comment` | Override the cluster root directory from configuration. |
| `--apps-files <list>` | `clusters`, `doctor`, `diff`, `comment` | Comma-separated apps files in precedence order, for example `apps.yaml,apps-dev.yaml`. |
| `--output-dir <path>` | `diff`, `comment` | Persist rendered manifests, split resources, diffs, warnings, errors, and summary artifacts. |
| `--context-lines <n>` | `diff`, `comment` | Number of unified diff context lines. |
| `--diff-mode raw|semantic|both` | `diff`, `comment` | Select raw manifest diff, semantic field diff, or both. |
| `--output-format plain|markdown` | `diff` | Select terminal-oriented plain output or standalone markdown output. |
| `--validate=true|false` | `diff`, `comment` | Enable or disable structural, schema, and semantic validation of current resources. |
| `--render-error-mode fail|warn-skip-release` | `diff`, `comment` | Fail on render errors, or skip broken releases and continue reporting the rest. |
| `--duplicate-key-mode error|warn-last-wins` | `diff`, `comment` | Treat duplicate YAML keys as errors, or warn and keep the last value. |

## GitLab Report Flags

These flags apply to `mobius comment`:

| Flag | Purpose |
| --- | --- |
| `--project-id <id>` | Override `CI_PROJECT_ID`. |
| `--mr-iid <iid>` | Override `CI_MERGE_REQUEST_IID`. |
| `--gitlab-base-url <url>` | Override `CI_API_V4_URL` or `CI_SERVER_URL`. |
| `--gitlab-token <token>` | Pass the GitLab API token directly. Prefer `GITLAB_API_TOKEN` in CI instead. |
| `--publish-target description|note` | Update the MR description, or publish a sticky MR note. |
| `--comment-mode full|summary|summary+artifacts` | Control report size and artifact reliance. |
| `--max-comment-bytes <n>` | Maximum GitLab report body size before falling back to a compact summary. |

For GitLab CI setup details, see [gitlab-ci.md](gitlab-ci.md).

## Examples

Inspect clusters:

```bash
mobius clusters
```

Run a local diff for one cluster:

```bash
mobius diff --cluster kube-bravo
```

Render standalone markdown:

```bash
mobius diff --cluster kube-bravo --output-format markdown
```

Use multiple apps files:

```bash
mobius diff --apps-files apps.yaml,apps-dev.yaml
```

Run against a single Helm chart repository:

```bash
mobius diff --chart-path . --values-files values.yaml,values-ci.yaml
```

Publish a GitLab MR report:

```bash
mobius comment \
  --base-ref "$CI_MERGE_REQUEST_TARGET_BRANCH_NAME" \
  --output-dir .mobius-out
```

In CI, provide a write-capable GitLab API token through `GITLAB_API_TOKEN`. `GITLAB_TOKEN` and `GITLAB_PRIVATE_TOKEN` are accepted aliases, but `CI_JOB_TOKEN` is not suitable for publishing MR descriptions or notes.
