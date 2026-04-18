# `flaxx check`

Query upstream Helm repositories and container registries for newer versions of the charts and images referenced by one or more apps. Read-only — `check` never mutates files.

## Synopsis

```text
flaxx check <cluster> [<app>] [flags]
```

Either pass an app name, or use `--all` to scan every app under the cluster. `<cluster>` is required.

## Flags

| Flag                | Default    | Description                                                                             |
| ------------------- | ---------- | --------------------------------------------------------------------------------------- |
| `-a`, `--all`       | `false`    | Scan every app under `<cluster>`. Mutually exclusive with an `<app>` positional arg.    |
| `--helm <name>`     | _none_     | Limit the check to specific Helm chart names. Repeatable: `--helm grafana --helm loki`. |
| `--stable`          | `false`    | Hide prereleases; only show stable versions as candidates.                              |
| `--include-pre`     | `false`    | Include prereleases alongside stable versions. Mutually exclusive with `--stable`.      |
| `--no-cache`        | `false`    | Skip reading from the cache (still writes fresh entries on each lookup).                |
| `--cache-ttl <dur>` | _config_   | Override the configured cache TTL for this run (e.g. `15m`, `2h`).                      |
| `-n`, `--namespace` | _app name_ | Override namespace used to locate app resources.                                        |

Global: `--config <path>`.

## Version filtering

By default, flaxx picks a sensible "release channel" per chart:

- If the pinned current version is stable (e.g. `1.2.3`), only stable candidates are surfaced.
- If the pinned current version is a prerelease (e.g. `1.2.3-rc.1`), prereleases are included.
- Unpinned releases get all versions.

Override with:

- `--stable` — force stable-only regardless of the current pin.
- `--include-pre` — force all candidates including prereleases.

## Caching

`flaxx check` caches upstream responses in `$XDG_CACHE_HOME/flaxx/` (fallback `~/.cache/flaxx/`) so repeated runs don't hit the network. Entries expire per the `cache.ttl` value in [`.flaxx.yaml`](../configuration.md#cache-settings) (default `1h`).

- `--no-cache` forces a fresh lookup and still writes the result, so a later `check` without the flag sees the new data.
- `--cache-ttl 30s` (or any `time.ParseDuration` value) overrides the configured TTL for one run.
- To wipe the cache completely, delete the directory.

## Examples

### One app, quick check

```bash
flaxx check production myapp
```

Example output:

```text
Helm: myapp
┌────────────────────────────────────┐
│ Chart:       myapp                 │
│ Repository:  https://charts.example.com │
│ Current:     1.0.0                 │
│ Latest:      1.4.2                 │
└────────────────────────────────────┘
  4 update(s) available:
  1.1.0
  1.2.0
  1.3.0
  1.4.2

Image: ghcr.io/example/myapp
┌───────────────────────────────────┐
│ Image:       ghcr.io/example/myapp│
│ Container:   myapp                │
│ Current:     v1.2.3               │
│ Latest:      v1.4.0               │
└───────────────────────────────────┘
  Up to date.
```

### Every app in a cluster

```bash
flaxx check production --all
```

### Filter by chart name

```bash
flaxx check production myapp --helm grafana --helm loki
```

Useful when an app has multiple HelmReleases but you only care about a subset.

### Stable channel only

```bash
flaxx check production myapp --stable
```

### Skip the cache for a single run

```bash
flaxx check production myapp --no-cache
```

### Custom TTL for CI

```bash
flaxx check production --all --cache-ttl 15m
```

## What gets scanned

- **Helm versions**: every HelmRelease in the app's cluster directory is matched to its HelmRepository (inline or referenced), and the upstream repository / OCI registry is queried.
- **Container images**: every Deployment / StatefulSet / DaemonSet under the app's namespace directory is walked; each container's image is probed against its registry. Registries speaking the OCI Distribution API are handled natively, including token auth and paginated tag listings.

`check` never modifies your repository. When you see a version you want to pick up, use [`flaxx update`](./update.md).

## Gotchas

- **Passing an `<app>` while using `--all` errors out.** Pick one or the other.
- **No apps found for `--all`** — `check` just prints `No apps found.` and exits cleanly. Check that `<cluster>` is correct and `<namespaces_dir>` is populated.
- **OCI registries without network access** will show per-chart errors at the end rather than failing the whole run; unrelated charts still get checked.
- **Prerelease detection** follows SemVer strictly (`-rc.1`, `-beta.2`, etc.). Non-SemVer tags on container images are tolerated but can't be ordered reliably; the "Latest" line is still populated based on whatever the registry lists first.

## See also

- [commands/update.md](./update.md) — apply a bump after `check` shows a candidate
- [configuration.md](../configuration.md#cache-settings) — cache configuration
