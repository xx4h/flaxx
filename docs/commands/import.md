# `flaxx import`

Adopt an app that is already running in a Kubernetes cluster into the flaxx repository. `import` is the inverse of `generate`: it reads live cluster state, sanitizes the manifests, and writes a complete flaxx-shaped app folder.

This is the only flaxx subcommand that talks to a live cluster. Kubeconfig loading follows the same rules as `kubectl` (honors `$KUBECONFIG`, current-context, and the `--kubeconfig`/`--context` flags).

## Synopsis

```text
flaxx import <cluster> <app> [flags]
```

`<cluster>` is the cluster directory name inside the repository (same semantics as `generate`/`switch`). `<app>` is both the namespace read from the live cluster and the folder created under `namespaces_dir/`. Use `-n, --namespace` to decouple the two.

## Flags

| Flag                | Default                             | Description                                                                                                            |
| ------------------- | ----------------------------------- | ---------------------------------------------------------------------------------------------------------------------- |
| `--kubeconfig`      | _`$KUBECONFIG` or `~/.kube/config`_ | Path to the kubeconfig used for the cluster read.                                                                      |
| `--context`         | _current context_                   | Kubeconfig context to use.                                                                                             |
| `-n`, `--namespace` | _app name_                          | Cluster namespace to read from. Independent of the folder name.                                                        |
| `--include-secrets` | `false`                             | Include `Secret` manifests in the adopted output (base64 data preserved as-is; encryption integration is future work). |
| `--helm-url`        | _auto-detect_                       | Helm repository URL. Required only when auto-detection cannot resolve the release's origin.                            |
| `--non-helm`        | `false`                             | Skip Helm detection even if a release secret exists; always emit raw manifests.                                        |
| `--force`           | `false`                             | Remove and overwrite an existing app directory.                                                                        |
| `--dry-run`         | `false`                             | Print what would be written; do not modify the repository.                                                             |

Global: `--config <path>`.

## What gets imported

`import` picks one of two output shapes:

### 1. Helm-managed → `HelmRelease` + `HelmRepository`

If a Secret of type `helm.sh/release.v1` named `sh.helm.release.v1.<app>.v<N>` exists in the namespace, flaxx:

1. Decodes the release (base64 → gzip → JSON).
2. Resolves the Helm repository URL (see precedence below).
3. Delegates to the same code path as `flaxx generate --type ext-helm` (or `--type ext-oci` when the URL starts with `oci://`), producing a `HelmRelease`, `HelmRepository`, `namespace.yaml`, `kustomization.yaml`, and the Flux `Kustomization`.

Repository URL precedence:

1. `--helm-url` flag.
2. Local Helm config: the first repository in `$HELM_REPOSITORY_CONFIG` (or `~/.config/helm/repositories.yaml`) whose cached index at `$HELM_REPOSITORY_CACHE` lists the chart+version.
3. Error with an actionable message pointing the user at `--helm-url` or `--non-helm`.

`oci://` URLs are detected automatically — the generated `HelmRepository` gets `type: oci` so Flux's source-controller talks to the registry correctly.

User-supplied values (what `helm get values <rel>` shows, without `--all`) are extracted from the release and written **inline** under `spec.values:` in the `HelmRelease`.

Chart defaults are deliberately _not_ duplicated — Flux resolves them from the chart at reconcile time, so unrelated upstream default changes keep tracking through instead of being frozen on adoption. An install with no overrides produces the existing `values: {}` placeholder.

No raw manifests are written — Flux will render the chart identically to the running release.

### 2. Raw manifests → per-resource YAML + `core` `Kustomization`

Otherwise, flaxx enumerates every user-facing namespaced resource in the namespace via API discovery, filters out the noise, sanitizes each object, and writes one YAML file per resource (`<kind>-<name>.yaml`). The resulting app folder is wrapped in a core-type Flux Kustomization — equivalent to what `flaxx generate --type core` scaffolds.

#### Filtered out automatically

- **Owned objects** (anything with `ownerReferences`): ReplicaSets from Deployments, Pods from ReplicaSets, Jobs from CronJobs, etc.
- **Ephemeral kinds**: `Event`, `Endpoints`, `EndpointSlice`, `Lease`, `PodMetrics`, `NodeMetrics`, `Pod`, `ControllerRevision`, `ReplicaSet`.
- **Helm release-state secrets** (`sh.helm.release.v1.*`).
- **Service-account tokens** (`type: kubernetes.io/service-account-token`).
- **Cluster-managed defaults**: the `default` ServiceAccount, the `kube-root-ca.crt` ConfigMap.
- **Secrets** — unless `--include-secrets` is passed.

#### Sanitized on every object

- `metadata`: `managedFields`, `resourceVersion`, `uid`, `generation`, `creationTimestamp`, `selfLink`, `deletionTimestamp`, `deletionGracePeriodSeconds`, `ownerReferences`.
- Annotations: `kubectl.kubernetes.io/last-applied-configuration`, `deployment.kubernetes.io/revision`.
- `status` block.

#### Sanitized per kind

| Kind                    | Stripped fields                                                                         |
| ----------------------- | --------------------------------------------------------------------------------------- |
| `Service`               | `spec.clusterIP`, `clusterIPs`, `ipFamilies`, `ipFamilyPolicy`, `internalTrafficPolicy` |
| `PersistentVolumeClaim` | `spec.volumeName`                                                                       |
| `ServiceAccount`        | `secrets` (auto-populated token refs)                                                   |
| Workloads + `CronJob`   | `spec.template.metadata.creationTimestamp` (and the nested one for CronJob)             |

## Examples

### Adopt a Helm-installed app

```bash
flaxx import production grafana --namespace monitoring
```

Produces a `HelmRelease` + `HelmRepository` pair, the in-namespace `kustomization.yaml`, `namespace.yaml`, and a Flux `Kustomization` pointing at them. The cluster-level `kustomization.yaml` is updated to reference the new app.

### Adopt raw manifests

```bash
flaxx import staging demo-app
```

One file per resource under `clusters/staging-namespaces/demo-app/`, wrapped in a core-type Flux `Kustomization` under `clusters/staging/`.

### Force raw-manifest output even if a Helm release exists

```bash
flaxx import production legacy-app --non-helm
```

Useful when the chart is no longer available, or when you want full control over the manifests without the Helm layer.

### Provide the Helm URL explicitly

```bash
flaxx import production grafana \
  --namespace monitoring \
  --helm-url https://grafana.github.io/helm-charts
```

Required when the chart wasn't installed from a repository that `helm repo list` knows about (for example, installed from a local tarball).

### Preview first

```bash
flaxx import production grafana --namespace monitoring --dry-run
```

Prints every file that would be written; nothing is touched on disk.

## Gotchas

- **No CRDs / no cluster-scoped resources.** `import` only enumerates namespaced resources. If the app includes a CRD or ClusterRole, write those manually (or bundle them with the app as an extra).
- **Secrets are not encrypted.** `--include-secrets` emits plain base64 — suitable only if your Git repository treats them securely. Plan on moving them to SOPS or SealedSecrets before pushing.
- **Re-apply will surface cluster-defaulted fields.** `kubectl diff -f clusters/.../demo-app/` will show cluster-assigned values (Service `clusterIP`, PVC `volumeName`, etc.) as drift — that's expected because they've been stripped on purpose. Flux won't reconcile them away because those fields are server-side defaults.
- **RBAC partial reads are tolerated.** If the import identity lacks permission on some API, that kind is skipped with a warning rather than failing the whole import.
- **Helm repository URL resolution relies on your local Helm config.** If the cluster's release came from a repository you never `helm repo add`-ed locally, auto-detect cannot find it — use `--helm-url`.
- **Discovery sees only what the API server advertises.** Aggregated APIs that are currently down will be skipped silently; run `kubectl api-resources` first if you suspect missing kinds.

## See also

- [commands/generate.md](./generate.md) — the forward direction: scaffold a brand-new app
- [commands/switch.md](./switch.md) — change workload kind after the fact
- [deploy-types.md](../deploy-types.md) — what `core` vs. `ext-helm` produce
