# `flaxx switch`

Convert the workload manifest of a `core`-type app between `Deployment`, `StatefulSet`, and `DaemonSet`. Adds / removes kind-specific fields automatically, renames the file on disk to match the new kind, and updates the namespace `kustomization.yaml`.

Only applies to `core`-type apps with raw Kubernetes manifests. Helm-based apps set the workload kind via chart values — `switch` does not edit HelmReleases.

## Synopsis

```text
flaxx switch <cluster> <app> --kind <kind> [flags]
```

## Flags

| Flag                | Default         | Description                                                                     |
| ------------------- | --------------- | ------------------------------------------------------------------------------- |
| `--kind`            | _(required)_    | Target kind: `deployment`, `statefulset`, or `daemonset`.                       |
| `--service-name`    | _metadata.name_ | For `statefulset` only: override the required `spec.serviceName`.               |
| `-n`, `--namespace` | _app name_      | Override namespace used to locate workload manifests.                           |
| `--dry-run`         | `false`         | Print the new YAML to stdout; do not write, rename, or touch the kustomization. |

Global: `--config <path>`.

## Transition matrix

| To &rarr;     | `replicas`           | `serviceName`                                      | `volumeClaimTemplates`     | `strategy` &harr; `updateStrategy` |
| ------------- | -------------------- | -------------------------------------------------- | -------------------------- | ---------------------------------- |
| `Deployment`  | ensured, default `1` | removed                                            | removed                    | `updateStrategy` &rarr; `strategy` |
| `StatefulSet` | ensured, default `1` | ensured (from `--service-name` or `metadata.name`) | ensured as `[]` if missing | `strategy` &rarr; `updateStrategy` |
| `DaemonSet`   | removed              | removed                                            | removed                    | `strategy` &rarr; `updateStrategy` |

`apiVersion: apps/v1` is enforced for all three kinds.

The `strategy` &harr; `updateStrategy` rename reuses the original value node, so any nested fields (e.g. `rollingUpdate.maxUnavailable`) carry across the rename unchanged.

## File rename

If the workload file follows the `<app>-<kind>.yaml` convention — which is what `flaxx generate --workload-kind` produces — `switch` renames it to match the new kind:

```text
before: clusters/production-namespaces/myapp/myapp-deployment.yaml
after:  clusters/production-namespaces/myapp/myapp-statefulset.yaml
```

The sibling `kustomization.yaml` is rewritten so the `resources:` list points at the new filename.

Non-conventional filenames (`workload.yaml`, `my-deploy.yaml`, etc.) are left alone; the content is updated in place.

## Examples

### Convert Deployment → StatefulSet

```bash
flaxx switch production myapp --kind statefulset
```

A notice goes to stderr noting that `serviceName` defaulted to the app's metadata name; pass `--service-name` if you want a different value (a headless Service name, typically).

### With an explicit serviceName

```bash
flaxx switch production myapp --kind statefulset --service-name myapp-headless
```

### Convert to DaemonSet

```bash
flaxx switch production myapp --kind daemonset
```

`replicas`, `serviceName`, and `volumeClaimTemplates` are removed; a `strategy:` block becomes `updateStrategy:`.

### Preview first

```bash
flaxx switch production myapp --kind daemonset --dry-run
```

Prints the proposed new YAML with an `(updated)` header. Nothing is written, nothing is renamed, the kustomization is untouched.

## Gotchas

- **Only one workload per app.** If the namespace dir contains more than one Deployment/StatefulSet/DaemonSet, `switch` refuses with `multiple workloads found in <dir>`. Split the app or pick a different workflow.
- **No workload found** — either the app is Helm-based (`switch` only handles raw manifests) or you haven't scaffolded a workload yet. Use `flaxx generate --workload-kind` first.
- **Identity switch is a no-op.** `--kind` equal to the current kind prints a notice and exits without writing.
- **Field preservation is not exhaustive.** The transition matrix handles the common kind-critical fields; anything else under `spec.` is left as-is. If your StatefulSet had `podManagementPolicy`, for example, it stays when switching to DaemonSet even though DaemonSet ignores it — review the diff.
- **Cluster-level `kustomization.yaml`** is not touched. It references the Flux Kustomization, not the workload file.

## See also

- [commands/generate.md](./generate.md#workload-kind) — scaffold a workload stub via `--workload-kind`
- [deploy-types.md](../deploy-types.md) — why this only applies to the `core` type
