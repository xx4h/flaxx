# `flaxx values`

Print the chart's default `values.yaml` for an app's HelmRelease — the full set of options the chart maintainer documents as overridable, with the comments that describe them. Useful for discovering what knobs a chart exposes before adding overrides to `spec.values` in your repository. Read-only — `values` never mutates files and never talks to the cluster.

## Synopsis

```text
flaxx values <cluster> <app> [flags]
```

`<cluster>` and `<app>` are both required. The app must have at least one `HelmRelease` in its cluster directory; raw-manifest apps (`-t core` without a chart) cannot be queried with `values`.

## Flags

| Flag              | Default                                                            | Description                                                                      |
| ----------------- | ------------------------------------------------------------------ | -------------------------------------------------------------------------------- |
| `--helm <chart>`  | _all releases_                                                     | Show values for the HelmRelease whose `spec.chart.spec.chart` matches this name. |
| `--version <ver>` | HelmRelease's `spec.chart.spec.version`, or _latest_ if not pinned | Override the chart version; useful for previewing the values of a different tag. |

Global: `--config <path>`.

## What gets printed

`values` discovers the HelmRelease(s) for the app the same way `check`, `inspect`, and `render` do, then for each one:

1. Reads `spec.chart.spec.chart`, `spec.chart.spec.version`, and `spec.chart.spec.sourceRef` from the HelmRelease.
2. Resolves the matching HelmRepository and pulls the chart (HTTPS or OCI) into Helm's local cache (`$XDG_CACHE_HOME/helm/repository/`).
3. Reads the chart's `values.yaml` verbatim — comments and ordering preserved — and writes it to stdout.

If the app has multiple HelmReleases and `--helm` isn't passed, every chart's `values.yaml` is printed, separated by `---`.

The output reflects the chart's own defaults — it does **not** include any overrides declared in your `spec.values` or `spec.valuesFrom`. To see the merged values that would actually be passed to the chart, use [`flaxx render --values-only`](./render.md).

## Examples

### Show defaults for the version pinned in the repository

```bash
flaxx values production myapp
```

### Inspect a different version without touching the repository

```bash
flaxx values production myapp --version 2.0.0
```

Useful when previewing what new options a chart upgrade would expose, or comparing the default values across two releases:

```bash
flaxx values production myapp --version 1.9.0 > /tmp/old.yaml
flaxx values production myapp --version 2.0.0 > /tmp/new.yaml
diff /tmp/old.yaml /tmp/new.yaml
```

### Pick one HelmRelease in a multi-release app

```bash
flaxx values production monitoring --helm grafana
```

Without `--helm`, every chart referenced by the app's HelmReleases is printed back-to-back.

## Caveats

- **Output is the chart's authored `values.yaml`** — comments, empty lines, and key ordering match the upstream file. It is **not** the merged values your HelmRelease would render with; for that, use `flaxx render --values-only`.
- **Chart caching is shared with the user's Helm CLI.** `values` writes into `$XDG_CACHE_HOME/helm/repository/` (or `~/.cache/helm/repository/`). A `helm repo update` you ran earlier helps populate that cache; a stale cache can hide a yanked version.
- **OCI registries with private auth.** `values` reuses the credentials in `$HELM_REGISTRY_CONFIG` (default `~/.config/helm/registry/config.json`). If you `helm registry login` first, `values` will pick it up. There is no flag for inline credentials.
- **Charts without a `values.yaml`** (rare — usually only minimal placeholder charts) print a warning on stderr and emit nothing on stdout.

## See also

- [commands/render.md](./render.md) — render the manifests a HelmRelease would produce, optionally with `--values-only` to see the merged values
- [commands/check.md](./check.md) — find newer chart versions to inspect
- [commands/update.md](./update.md) — apply a chart version bump after reviewing the new defaults
