# Deploy types

Every `flaxx generate` call picks a deploy type with `-t / --type`. The type decides which source resources Flux will use to find the app's manifests (local, external Git, Helm chart repository, or OCI registry) and therefore which files flaxx scaffolds.

There are five types:

| Type        | Source for app manifests                     | Generates in cluster dir                                | Use when                                                                       |
| ----------- | -------------------------------------------- | ------------------------------------------------------- | ------------------------------------------------------------------------------ |
| `core`      | Local YAML under `<namespaces>/<app>/`       | Flux Kustomization                                      | You write / paste raw Kubernetes manifests into your Flux repository           |
| `core-helm` | Local + Helm chart (repository defined here) | Flux Kustomization + HelmRepository + HelmRelease       | You install a chart from a repository you define inline                        |
| `ext-git`   | External Git repository                      | Flux Kustomization (dual) + GitRepository               | The app's manifests live in a separate Git repository you don't want to vendor |
| `ext-helm`  | Helm chart from an external HTTP repository  | Flux Kustomization + HelmRepository + HelmRelease       | You install a chart from a well-known upstream (Bitnami, Grafana, etc.)        |
| `ext-oci`   | Helm chart from an OCI registry              | Flux Kustomization + HelmRepository (OCI) + HelmRelease | You install a chart distributed via OCI (ghcr.io, Harbor, etc.)                |

Regardless of type, flaxx always creates the namespace directory (`namespace.yaml` + `kustomization.yaml`) and updates the parent `kustomization.yaml` in the cluster dir.

All examples below assume the default flat layout (`cluster_subdirs: false`, `cluster_dir: clusters/{{.Cluster}}`, `namespaces_dir: clusters/{{.Cluster}}-namespaces`).

## `core`

Minimal scaffolding. Flux applies whatever raw YAML you place under `<namespaces>/<app>/`.

```bash
flaxx generate production myapp -t core
```

```text
clusters/production/
├── kustomization.yaml              # auto-updated with the line below
└── myapp-kustomization.yaml        # Flux Kustomization targeting the namespace dir

clusters/production-namespaces/myapp/
├── kustomization.yaml              # resources: [namespace.yaml]
└── namespace.yaml
```

The namespace `kustomization.yaml` only lists `namespace.yaml` until you add more files — either by dropping them in yourself or using `-e <extra>` / `--workload-kind <kind>` at generate time, or `flaxx add` later.

### With a workload stub

```bash
flaxx generate production myapp -t core --workload-kind statefulset
```

Adds `<app>-statefulset.yaml` and registers it in the namespace `kustomization.yaml`:

```text
clusters/production-namespaces/myapp/
├── kustomization.yaml        # includes: - myapp-statefulset.yaml
├── namespace.yaml
└── myapp-statefulset.yaml    # apps/v1 StatefulSet, one placeholder container
```

See [commands/generate.md](./commands/generate.md#workload-kind) and [commands/switch.md](./commands/switch.md) for details.

## `core-helm`

Like `core`, plus a HelmRepository and HelmRelease in a single combined YAML file. Useful when you're installing a chart from a repository you define inline (or your own private chart museum).

```bash
flaxx generate production myapp -t core-helm \
  --helm-url https://charts.example.com
```

```text
clusters/production/
├── kustomization.yaml
├── myapp-kustomization.yaml
└── myapp-helm.yml              # HelmRepository + HelmRelease

clusters/production-namespaces/myapp/
├── kustomization.yaml
└── namespace.yaml
```

Flags:

- `--helm-url` (required) — the Helm repository URL.
- `--helm-chart` — chart name (defaults to the app name).
- `--helm-version` — pins the chart to a specific version (optional; unpinned means Flux follows the chart repository's latest).

## `ext-git`

Dual Kustomization pattern: one Flux Kustomization does the local namespace setup, a second one pulls the app's own manifests from an external Git repository. Great when an app already maintains its own GitOps-ready manifests in a separate repository.

```bash
flaxx generate production myapp -t ext-git \
  --git-url https://github.com/org/myapp.git \
  --git-branch main \
  --git-path ./deploy/production \
  --git-secret git-repo-secret
```

```text
clusters/production/
├── kustomization.yaml
├── myapp-kustomization.yaml     # TWO Kustomizations in one file:
│                                #   1. local namespace setup
│                                #   2. app manifests from the external repo
└── myapp-git.yml                # GitRepository(myapp)

clusters/production-namespaces/myapp/
├── kustomization.yaml
└── namespace.yaml
```

Flags:

- `--git-url` (required) — Git repository URL.
- `--git-branch` — branch to track. Default `main`.
- `--git-path` — path inside the repository containing the manifests. Default `./deploy/production`.
- `--git-secret` — name of a Kubernetes Secret holding Git auth credentials. Default `git-repo-secret`.

The secret must exist in the `flux-system` namespace; see Flux's [GitRepository spec docs](https://fluxcd.io/flux/components/source/gitrepositories/) for generating it.

## `ext-helm`

Helm chart from an external HTTP chart repository. The most common pick for "install a well-known chart."

```bash
flaxx generate production podinfo -t ext-helm \
  --helm-url https://stefanprodan.github.io/podinfo \
  --helm-chart podinfo \
  --helm-version 6.5.4
```

```text
clusters/production/
├── kustomization.yaml
├── podinfo-kustomization.yaml
└── podinfo-helm.yml            # HelmRepository + HelmRelease

clusters/production-namespaces/podinfo/
├── kustomization.yaml
└── namespace.yaml
```

Same flag set as `core-helm`. The scaffolded HelmRelease has `values: {}` — edit it to override chart values; `flaxx update` preserves the block when bumping the version.

## `ext-oci`

Helm chart from an OCI registry. The URL must begin with `oci://`.

```bash
flaxx generate production myapp -t ext-oci \
  --helm-url oci://ghcr.io/example/charts \
  --helm-chart myapp \
  --helm-version 1.2.3
```

Produces the same tree as `ext-helm`, but the HelmRepository is marked `type: oci`. Upstream discovery uses the OCI Distribution spec (including token auth and pagination) — see [commands/check.md](./commands/check.md) for how `flaxx check` handles OCI registries.

## Choosing a type

- You have raw manifests you edit directly → **`core`**.
- You install one Helm chart per app from a well-known repository → **`ext-helm`** (or **`ext-oci`** if it's OCI-distributed).
- You run your own chart museum → **`core-helm`**.
- The app's manifests live in another repository and you don't want to vendor them → **`ext-git`**.

Mixing types across a single cluster is fine — flaxx's `kustomization.yaml` in the cluster dir collects whatever is generated regardless of type.

## See also

- [commands/generate.md](./commands/generate.md) — every `generate` flag
- [configuration.md](./configuration.md#two-common-layout-presets) — how path templating and flat-vs-subdirs interact with each type
- [commands/check.md](./commands/check.md) / [commands/update.md](./commands/update.md) — maintenance for scaffolded apps
- [commands/show.md](./commands/show.md) — render the manifests a `core-helm` / `ext-helm` / `ext-oci` HelmRelease would produce
