# `flaxx config`

Two subcommands for working with `.flaxx.yaml`:

- `flaxx config show` — print the effective config to stdout, read-only.
- `flaxx config init` — scan the current directory for a Flux repository and write a matching `.flaxx.yaml`.

Neither subcommand takes positional args or flags beyond the global `--config <path>`.

## `flaxx config show`

Prints the effective configuration as YAML — whatever the currently loaded `.flaxx.yaml` says, merged with defaults for any omitted fields. If no config file is found, it prints the pure defaults (a convenient way to see what flaxx would do out of the box).

```bash
flaxx config show
```

```yaml
defaults:
  interval: 2m
  timeout: 1m
  prune: false
paths:
  cluster_dir: clusters/{{.Cluster}}
  namespaces_dir: clusters/{{.Cluster}}-namespaces
  cluster_subdirs: false
naming:
  kustomization: "{{.App}}-kustomization.yaml"
  helm: "{{.App}}-helm.yml"
  git: "{{.App}}-git.yml"
  namespace: namespace.yaml
  ns_kustomization: kustomization.yaml
cache:
  enabled: true
  ttl: 1h
templates_dir: .flaxx/templates
```

Useful in CI: `flaxx config show | yq …` to assert settings, or to diff against a checked-in baseline.

## `flaxx config init`

Scans the current directory for common Flux layouts and writes a matching `.flaxx.yaml`.

```bash
flaxx config init
# Detected structure:
#   Cluster dir:    clusters/{{.Cluster}}/apps
#   Namespaces dir: apps
#   Layout:         flat
#   Clusters:       production, staging
#
# Created .flaxx.yaml
```

### Detection strategy

Tried in order, first match wins:

1. `clusters/*/apps` + `apps` (standard Flux layout — [fluxcd/flux2-kustomize-helm-example](https://github.com/fluxcd/flux2-kustomize-helm-example))
2. `clusters/*` + `apps`
3. `clusters/*` + `clusters/*-namespaces` (flaxx default)
4. `flux/clusters/*/apps` + `flux/apps`
5. `flux/clusters/*` + `flux/clusters/*-namespaces`
6. Fallback: walk the tree (max depth 4) looking for any directory containing `*-kustomization.yaml`.

The detector confirms a candidate by checking for `*-kustomization.yaml` files or subdirectories that look like app subdirs. Patterns like `clusters/*` that match multiple clusters are accepted and the cluster names are reported in the output.

Glob wildcards in the matched path are converted back to flaxx templates — `clusters/*` becomes `clusters/{{.Cluster}}`, `clusters/*-namespaces` becomes `clusters/{{.Cluster}}-namespaces`.

### Gotchas

- **Won't overwrite an existing `.flaxx.yaml`**: you get `.flaxx.yaml already exists; remove it first to regenerate`. Delete or rename the file before re-running.
- **Fallback results can be weird** when no known pattern matches — the first directory containing a `*-kustomization.yaml` is reported and used for both `cluster_dir` and `namespaces_dir`. Review the generated file before committing.
- **No flux repository detected** errors out with `no flux repository structure detected`. Start from scratch with `flaxx generate` to create the structure (it lays down the defaults).

## See also

- [configuration.md](../configuration.md) — full schema and field semantics
- [commands/inspect.md](./inspect.md) — verify the config against a real repository
