# `flaxx update`

Bump an app's Helm chart version or container image in place. Mutates the scaffolded YAML directly — comments and unrelated fields are preserved.

## Synopsis

```text
flaxx update <cluster> <app> [--helm chart:version ...] [--image [name=]image:tag] [flags]
```

At least one of `--helm`, `--helm-version`, or `--image` must be provided.

## Flags

| Flag                 | Default    | Description                                                                                           |
| -------------------- | ---------- | ----------------------------------------------------------------------------------------------------- |
| `--helm <c>:<v>`     | —          | Update a Helm chart version. Format: `chart:version`. Repeatable for multi-chart apps.                |
| `--helm-version <v>` | —          | **Deprecated.** Prefer `--helm`. Updates a single HelmRelease to the given version.                   |
| `--image <spec>`     | —          | Update a container image. Format: `image:tag` or `container-name=image:tag` for multi-container pods. |
| `-n`, `--namespace`  | _app name_ | Override namespace used to locate workload manifests.                                                 |
| `--dry-run`          | `false`    | Print the updated YAML to stdout instead of writing it back.                                          |

`--helm` and `--helm-version` are mutually exclusive. Global: `--config <path>`.

Shell completions for `--helm-version` and `--image` query upstream registries for candidate values, so you can press `<TAB>` after the flag and get a list of actual versions / tags to pick from.

## Examples

### Bump a single Helm chart

```bash
flaxx update production myapp --helm myapp:2.0.0
```

### Bump multiple charts in one call

```bash
flaxx update production myapp --helm grafana:8.0.0 --helm loki:3.0.0
```

Each `--helm` matches against `spec.chart.spec.chart` inside the HelmRelease.

### Update a container image

```bash
flaxx update production myapp --image registry/myapp:v1.2.3
```

### Target one container in a multi-container pod

```bash
flaxx update production myapp --image sidecar=registry/sidecar:v2.0
```

Without the `<name>=` prefix, `update` updates the first matching container; with the prefix it only matches the one whose `name:` equals the prefix.

### Preview without writing

```bash
flaxx update production myapp --helm myapp:2.0.0 --dry-run
```

Prints the full post-update YAML with a `(updated)` header. No files are touched.

## How matching works

- **Helm**: walks every `*.yaml` / `*.yml` in the app's cluster directory, finds any `HelmRelease`, matches `--helm <name>:<version>` against `spec.chart.spec.chart`. An unmatched chart name returns an error so nothing silently no-ops.
- **Image**: walks the app's namespaces directory for any Deployment / StatefulSet / DaemonSet, then walks `spec.template.spec.containers[*]`. With `--image name=image:tag`, only the container whose `name:` equals `name` is updated; without the prefix, the first container wins.

## Gotchas

- **Deprecated flag**: `--helm-version` still works but only when there's exactly one HelmRelease in the app's cluster directory. Use `--helm chart:version` everywhere else — it also reads better in Git history.
- **YAML formatting is preserved** (gopkg.in/yaml.v3 node-based rewriting). Comments, field ordering, and quoting style survive the update. However, flow-style edge cases may be re-flowed; diff before committing if your repository has strict style expectations.
- **`--helm` and `--helm-version` cannot be combined in one call.** flaxx enforces this with `MarkFlagsMutuallyExclusive`.
- **Missing apps**: if the app's cluster dir / namespaces dir doesn't exist, the command returns an error — `update` can only edit apps scaffolded by `generate`.

## See also

- [commands/check.md](./check.md) — find out which versions are available to bump to
- [commands/generate.md](./generate.md) — scaffold apps so there's something to update
