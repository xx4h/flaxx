# `flaxx show`

Render the Kubernetes manifests an app's HelmRelease would produce. Pulls the pinned chart, layers in the values declared in your repository (and any CLI overrides), and prints the rendered YAML — equivalent to `helm template`, but driven by the Flux files instead of an ad-hoc `helm install` invocation. Read-only — `show` never mutates files and never talks to the cluster.

## Synopsis

```text
flaxx show <cluster> <app> [flags]
```

`<cluster>` and `<app>` are both required. The app must have at least one `HelmRelease` in its cluster directory; raw-manifest apps (`-t core` without a chart) cannot be rendered with `show`.

## Flags

| Flag                 | Default                       | Description                                                                                                         |
| -------------------- | ----------------------------- | ------------------------------------------------------------------------------------------------------------------- |
| `-n`, `--namespace`  | HelmRelease's namespace       | Override the release namespace passed to the chart.                                                                 |
| `--helm <chart>`     | _all releases_                | Render only the HelmRelease whose `spec.chart.spec.chart` matches this name. Required when an app declares several. |
| `--release-name <n>` | HelmRelease's `metadata.name` | Override the release name passed to the chart.                                                                      |
| `-f`, `--values <p>` | _none_                        | Values file merged on top of the HelmRelease's values. Repeatable.                                                  |
| `--set <k=v>`        | _none_                        | Set a single value on the command line. Repeatable. Layered after `-f`.                                             |
| `--set-string <k=v>` | _none_                        | Like `--set`, but every value is forced to a string. Repeatable.                                                    |
| `--skip-crds`        | `false`                       | Drop the chart's `crds/` contents from the rendered output.                                                         |
| `--values-only`      | `false`                       | Print the merged values that would be passed to the chart, instead of the rendered manifests.                       |
| `--kube-version <v>` | `v1.30.0`                     | Kubernetes version reported to the chart for `.Capabilities.KubeVersion` and `kubeVersion:` constraint checks.      |
| `--api-versions <v>` | _none_                        | Extra API versions added to `.Capabilities.APIVersions` (e.g. `monitoring.coreos.com/v1`). Repeatable.              |

Global: `--config <path>`.

## What gets rendered

`show` discovers the HelmRelease(s) for the app the same way `check` and `inspect` do, then for each one:

1. Reads `spec.chart.spec.chart`, `spec.chart.spec.version`, and `spec.chart.spec.sourceRef` from the HelmRelease.
2. Resolves the matching HelmRepository and pulls the chart (HTTPS or OCI) into Helm's local cache (`$XDG_CACHE_HOME/helm/repository/`).
3. Builds the values map by layering, in this order:

- Each `spec.valuesFrom` entry, in declaration order — see [valuesFrom resolution](#valuesfrom-resolution).
- Inline `spec.values`.
- Files from `-f`/`--values`, in order.
- `--set` and `--set-string` overrides, in order.

4. Runs the Helm template engine client-side (no cluster contact, no hooks, no install) and writes the result to stdout.

If the app has multiple HelmReleases and `--helm` isn't passed, every release is rendered, separated by `---`.

### valuesFrom resolution

Flux normally fetches the ConfigMap or Secret named in `spec.valuesFrom` from the cluster at reconcile time. `show` runs offline, so it can't do that — instead it scans the app's namespaces directory for a matching resource on disk:

- `kind` and `metadata.name` must match.
- The default key is `values.yaml`; pass `valuesKey:` in the HelmRelease to use another.
- Both `data` and `stringData` are supported (Secrets often use `stringData`).
- `optional: true` entries that aren't found are skipped with a warning on stderr.
- Required entries that aren't found are an error.

This is intentionally pragmatic: if the resource lives under your app's namespace directory (the typical pattern), `show` finds it. If the values come from a sealed Secret or a runtime-rendered ConfigMap, you'll either need to materialize a plaintext copy alongside or override with `-f`/`--set`.

## Examples

### Preview a HelmRelease

```bash
flaxx show production myapp
```

### Inspect just the merged values

When you want to verify _what_ gets passed to the chart without wading through a multi-thousand-line manifest:

```bash
flaxx show production myapp --values-only
```

```yaml
replicaCount: 2
image:
  repository: ghcr.io/example/myapp
  tag: v1.4.0
ingress:
  enabled: true
  hosts:
    - myapp.example.com
```

### Try out a CLI override before committing it

```bash
flaxx show production myapp --set replicaCount=5 -f local-overrides.yaml
```

The override is applied on top of the values declared in the HelmRelease, mirroring how a future `--set` in your repository would land. The repository on disk is unchanged.

### Pick one HelmRelease in a multi-release app

```bash
flaxx show production monitoring --helm grafana
```

Without `--helm`, both `grafana` and `loki` would be rendered back-to-back.

### Diff against the live cluster

`show` produces stable output, so a quick way to spot drift:

```bash
flaxx show production myapp > /tmp/desired.yaml
kubectl get -n myapp -o yaml deploy,svc,ingress > /tmp/live.yaml
diff <(yq -y . /tmp/desired.yaml) <(yq -y . /tmp/live.yaml)
```

(Trim runtime fields like `status:` / `managedFields:` first if you want the diff to be readable — same caveat as `flaxx import`.)

### Render against an older Kubernetes API surface

Some charts gate templates on the Kubernetes minor version. To preview what the chart would do on an older cluster:

```bash
flaxx show production cert-manager --kube-version v1.27.0
```

### Tell the chart that a CRD is installed

Charts that use `.Capabilities.APIVersions.Has` to decide whether to emit a `ServiceMonitor` / `PrometheusRule` / etc. won't see your real cluster's APIs in client-only mode. Add them explicitly:

```bash
flaxx show production myapp \
  --api-versions monitoring.coreos.com/v1 \
  --api-versions cert-manager.io/v1
```

## Caveats

- **No cluster contact.** Hooks (`helm.sh/hook`) are disabled. Templates that reference `lookup` of cluster state will get the empty-result behavior Helm uses in client-only mode. If a chart genuinely requires the cluster to render correctly, `show` will be misleading — but the same is true of `helm template`.
- **Values precedence matches `helm`, not Flux's `defaults` / `overrides` knobs.** `show` does not implement the `valuesKey: <key>` + `optional` + `targetPath: <jsonpath>` combination from Flux's HelmRelease spec — `targetPath` (which writes a single value into a deep path) is ignored. The other fields are honored.
- **Chart caching is shared with the user's Helm CLI.** `show` writes into `$XDG_CACHE_HOME/helm/repository/` (or `~/.cache/helm/repository/`). A `helm repo update` you ran earlier helps populate that cache; a stale cache can hide a yanked version.
- **OCI registries with private auth.** `show` reuses the credentials in `$HELM_REGISTRY_CONFIG` (default `~/.config/helm/registry/config.json`). If you `helm registry login` first, `show` will pick it up. There is no flag for inline credentials.
- **Default `kubeVersion` is `v1.30.0`** — newer than Helm's own default of `v1.20.0`, because most modern charts refuse to render against 1.20. Override with `--kube-version` if you need a different target.

## Gotchas

- **An app with multiple HelmReleases without `--helm`** renders all of them concatenated. That's usually fine for grep-ing and for diffing, but if you wanted just one, pass `--helm <chart>`.
- **`--values-only` ignores `--skip-crds`** — CRDs aren't part of the values map, so the flag has no effect there.
- **`spec.valuesFrom` referencing a sealed Secret** (`SealedSecret` / `bitnami-labs/sealed-secrets`) cannot be decrypted offline. The lookup will fail unless the underlying `Secret` has been materialized into the namespace directory next to the sealed copy.
- **Charts that pull subchart dependencies from a private repository** need `helm dependency update` to have been run for the chart to be cacheable. `show` does not run `dependency update` for you — pre-warm with the Helm CLI if needed.

## See also

- [commands/check.md](./check.md) — find newer chart / image versions
- [commands/update.md](./update.md) — apply a version bump
- [commands/inspect.md](./inspect.md) — list which HelmReleases an app declares
