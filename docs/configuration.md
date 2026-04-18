# Configuration

flaxx is driven by a YAML config file — `.flaxx.yaml` — placed at the root of your Flux repository. The file is optional: if missing, flaxx uses sensible defaults that match the Flux-recommended flat layout.

## How the config is found

1. If `--config <path>` is passed (a global flag), that file is loaded.
2. Otherwise flaxx walks up from the current working directory looking for the first `.flaxx.yaml` it finds.
3. If nothing is found, defaults apply.

This means any flaxx command run anywhere inside your repository picks up the same config without needing to `cd` to the root.

## Generating a config

Two options:

```bash
# Auto-detect from the existing directory structure and write .flaxx.yaml
flaxx config init

# Just print what the effective config would be (writes nothing)
flaxx config show
```

`config init` inspects your tree for common Flux layouts — `clusters/<cluster>/apps/` + `apps/`, `clusters/<cluster>/` + `clusters/<cluster>-namespaces/`, or the flaxx default — and produces a matching `.flaxx.yaml`. See [commands/config.md](./commands/config.md) for details on the detection.

## Full schema

All fields are optional. Any field you omit falls back to the default shown here.

```yaml
defaults:
  interval: "2m" # Flux reconciliation interval for the Kustomization
  timeout: "1m" # Flux sync timeout
  prune: false # Enable `spec.prune: true` on generated Kustomizations

paths:
  cluster_dir: "clusters/{{.Cluster}}" # Where cluster-level files (Flux Kustomization, HelmRelease, GitRepository) live
  namespaces_dir: "clusters/{{.Cluster}}-namespaces" # Where per-app namespace + workload files live
  cluster_subdirs: false # false = flat (files named <app>-*.yaml), true = per-app subdirectory

naming:
  kustomization: "{{.App}}-kustomization.yaml" # Filename for the Flux Kustomization
  helm: "{{.App}}-helm.yml" # Filename for the combined HelmRepository + HelmRelease
  git: "{{.App}}-git.yml" # Filename for the GitRepository (ext-git only)
  namespace: "namespace.yaml" # Filename for the Namespace resource
  ns_kustomization: "kustomization.yaml" # Filename for the namespace-level Kustomize resource list

cache:
  enabled: true # Cache upstream registry responses for `flaxx check`
  ttl: "1h" # Cache TTL (any duration parseable by Go's time.ParseDuration)

templates_dir: ".flaxx/templates" # Where custom extras live (see extras.md)
```

## Path templating

Fields under `paths:` and `naming:` are [Go `text/template`](https://pkg.go.dev/text/template) strings. These variables are available:

| Variable         | Value                                                       |
| ---------------- | ----------------------------------------------------------- |
| `{{.Cluster}}`   | The cluster name passed as the first positional arg         |
| `{{.App}}`       | The app name passed as the second positional arg            |
| `{{.Namespace}}` | Resolved namespace (app name, or `-n/--namespace` override) |

A simple path like `clusters/{{.Cluster}}` becomes `clusters/production` when you run `flaxx generate production myapp …`. A path without any `{{ }}` placeholders is used verbatim.

## Layout: flat vs. subdirs

`paths.cluster_subdirs` controls how files in the cluster directory are organized.

### Flat (`cluster_subdirs: false`, default)

Each app contributes `<app>-*.yaml` files directly into the cluster directory:

```text
clusters/production/
├── kustomization.yaml          # auto-managed resource list
├── myapp-kustomization.yaml    # Flux Kustomization (generated)
├── myapp-helm.yml              # HelmRepository + HelmRelease (if helm-y type)
├── otherapp-kustomization.yaml
└── otherapp-git.yml

clusters/production-namespaces/myapp/
├── kustomization.yaml
├── namespace.yaml
└── (workload files go here)
```

### Subdirs (`cluster_subdirs: true`)

Each app gets its own subdirectory under the cluster dir:

```text
clusters/production/
├── kustomization.yaml
├── myapp/
│   ├── myapp-kustomization.yaml
│   └── myapp-helm.yml
└── otherapp/
    ├── otherapp-kustomization.yaml
    └── otherapp-git.yml

clusters/production-namespaces/myapp/
├── kustomization.yaml
├── namespace.yaml
└── (workload files go here)
```

The namespaces directory is always per-app regardless of `cluster_subdirs`.

## Two common layout presets

### Flaxx default

```yaml
paths:
  cluster_dir: "clusters/{{.Cluster}}"
  namespaces_dir: "clusters/{{.Cluster}}-namespaces"
  cluster_subdirs: false
```

Produces `clusters/<cluster>/` and `clusters/<cluster>-namespaces/<app>/`. Matches the Flux-recommended flat layout.

### Flux-official `apps/` layout

```yaml
paths:
  cluster_dir: "clusters/{{.Cluster}}/apps"
  namespaces_dir: "apps"
  cluster_subdirs: false
```

Produces `clusters/<cluster>/apps/` and `apps/<app>/`. Matches the [fluxcd/flux2-kustomize-helm-example](https://github.com/fluxcd/flux2-kustomize-helm-example) structure. `flaxx config init` detects this pattern automatically.

## Cache settings

`flaxx check` queries upstream Helm repositories and container registries. Those responses are cached so that repeated checks (in CI, or during development) don't hammer the upstreams.

```yaml
cache:
  enabled: true
  ttl: "15m" # any time.ParseDuration value: 30s, 5m, 2h, 24h, …
```

- Cache directory: `$XDG_CACHE_HOME/flaxx` (falls back to `~/.cache/flaxx`).
- `--no-cache` on the `check` command forces refresh (still writes new entries).
- `--cache-ttl <duration>` on the `check` command overrides the configured TTL for one run.

## Templates directory

```yaml
templates_dir: ".flaxx/templates"
```

flaxx discovers user-authored extras under this directory. Each subdirectory with a `_meta.yaml` is an extra. See [extras.md](./extras.md) for the full format.

## Defaults reference

If you write no config at all, flaxx behaves as if this file existed:

```yaml
defaults:
  interval: "2m"
  timeout: "1m"
  prune: false

paths:
  cluster_dir: "clusters/{{.Cluster}}"
  namespaces_dir: "clusters/{{.Cluster}}-namespaces"
  cluster_subdirs: false

naming:
  kustomization: "{{.App}}-kustomization.yaml"
  helm: "{{.App}}-helm.yml"
  git: "{{.App}}-git.yml"
  namespace: "namespace.yaml"
  ns_kustomization: "kustomization.yaml"

cache:
  enabled: true
  ttl: "1h"

templates_dir: ".flaxx/templates"
```

## See also

- [deploy-types.md](./deploy-types.md) — which files each deploy type produces
- [commands/config.md](./commands/config.md) — `flaxx config show` and `flaxx config init` details
- [commands/inspect.md](./commands/inspect.md) — verify the effective config against a real repository
