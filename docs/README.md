# flaxx documentation

flaxx is a scaffolding and maintenance tool for FluxCD GitOps repositories. This directory contains the reference docs and a full end-to-end walkthrough.

If you are brand new to flaxx, read the [walkthrough](./walkthrough.md) first — it takes you from an empty Git repository to a running Flux-managed cluster with multiple apps and exercises every flaxx feature along the way.

## Where to start

| If you want to…                          | Read                                   |
| ---------------------------------------- | -------------------------------------- |
| See flaxx in action end-to-end           | [walkthrough.md](./walkthrough.md)     |
| Install the binary                       | [installation.md](./installation.md)   |
| Understand the `.flaxx.yaml` config      | [configuration.md](./configuration.md) |
| Pick the right deploy type for a new app | [deploy-types.md](./deploy-types.md)   |
| Write a reusable template (an _extra_)   | [extras.md](./extras.md)               |
| Look up a specific subcommand            | [commands/](./commands/)               |

## Command reference

Per-command pages, one per subcommand — purpose, flags, examples, gotchas.

- [`flaxx generate`](./commands/generate.md) — scaffold a new Flux app
- [`flaxx import`](./commands/import.md) — adopt an app already running in a cluster
- [`flaxx add`](./commands/add.md) — layer extras onto an existing app
- [`flaxx switch`](./commands/switch.md) — migrate a workload between Deployment / StatefulSet / DaemonSet
- [`flaxx update`](./commands/update.md) — bump a Helm chart version or container image
- [`flaxx check`](./commands/check.md) — query upstreams for newer versions
- [`flaxx render`](./commands/render.md) — render the manifests a HelmRelease would produce
- [`flaxx values`](./commands/values.md) — print the default values.yaml of an app's chart
- [`flaxx inspect`](./commands/inspect.md) — describe what flaxx sees in the current repository
- [`flaxx config`](./commands/config.md) — show or initialize `.flaxx.yaml`
- [`flaxx template`](./commands/template.md) — list / initialize / extract extras

## Concept pages

- [Installation](./installation.md) — Nix, Homebrew, Go, from source, shell completions
- [Configuration](./configuration.md) — every field in `.flaxx.yaml`, with flat vs. subdirs layouts
- [Deploy types](./deploy-types.md) — `core`, `core-helm`, `ext-git`, `ext-helm`, `ext-oci`: what each produces, when to pick which
- [Extras](./extras.md) — `_meta.yaml` schema, variable resolution, built-ins, authoring custom extras

## How the docs relate to the main `README.md`

The top-level [`README.md`](../README.md) is the short pitch and quickstart. This `docs/` tree is where the depth lives: every flag, every edge case, a realistic worked example. If a fact contradicts between the two, the docs here are authoritative — the top-level file is condensed.
