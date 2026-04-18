# `flaxx inspect`

Describe what flaxx sees when it looks at the current repository: which config file is active, which paths are resolved, which clusters exist, which apps live in each cluster, and a one-line summary of the files / Helm releases / container images found per app.

Read-only. Good as a first thing to run when you `cd` into an unfamiliar repository.

## Synopsis

```text
flaxx inspect
```

No positional args, no local flags. Global `--config <path>` applies.

## What you see

Three sections:

1. **Configuration** — which config file was loaded (or "(none, using defaults)"), the resolved `cluster_dir` / `namespaces_dir` templates, the detected layout, and the templates directory.
2. **Per-cluster**: path pair, layout (`flat`, `subdirs`, `mixed`, `empty`), count of apps.
3. **Per-app** (inside each cluster): the list of files in the cluster dir, the list of files in the namespace dir, and — if detectable — the Helm chart with current version and repository URL, plus every container image found.

Output is styled (lipgloss) when attached to a terminal: bold app names, dim URLs, yellow for Helm, magenta for container images. In a non-TTY pipe the colours strip out cleanly.

## Example

```text
flaxx inspect

Configuration
┌─────────────────────────────────────────────────────────────────┐
│ Config file:    .flaxx.yaml                                     │
│ Cluster dir:    clusters/{{.Cluster}}                           │
│ Namespaces dir: clusters/{{.Cluster}}-namespaces                │
│ Layout:         flat                                            │
│ Templates dir:  .flaxx/templates                                │
└─────────────────────────────────────────────────────────────────┘

Cluster: production
┌─────────────────────────────────────────────────────────────────┐
│ Paths:    clusters/production, clusters/production-namespaces  │
│ Layout:   flat                                                  │
│ Apps:     3                                                     │
└─────────────────────────────────────────────────────────────────┘

  podinfo
    cluster:
      podinfo-kustomization.yaml
      podinfo-helm.yml
    namespace:
      kustomization.yaml
      namespace.yaml
    helm:  podinfo 6.5.4 (https://stefanprodan.github.io/podinfo)
    image: podinfod ghcr.io/stefanprodan/podinfo:6.5.4
  ...
```

## Layout detection

For each cluster dir, `inspect` reports one of:

| Layout    | Meaning                                                                                   |
| --------- | ----------------------------------------------------------------------------------------- |
| `flat`    | Only `<app>-kustomization.yaml` style files — no per-app subdirectories.                  |
| `subdirs` | Only per-app subdirectories — no flat-style files.                                        |
| `mixed`   | Both patterns present. Usually an accidental mix; consider picking one and migrating.     |
| `empty`   | Neither pattern found. Fresh cluster dir or an unrelated layout flaxx couldn't recognize. |

`flux-system/` (the Flux bootstrap output) is ignored when classifying.

## Gotchas

- **Missing cluster dir**: if `config.paths.cluster_dir` resolves to a path that doesn't exist, `inspect` prints "No clusters found." — verify the config with `flaxx config show`.
- **Per-cluster iteration order is unsorted** (Go map range). If you care about deterministic output, pipe through `sort`.

## See also

- [commands/config.md](./config.md) — show or re-initialize `.flaxx.yaml`
- [configuration.md](../configuration.md) — how paths and layout are resolved
