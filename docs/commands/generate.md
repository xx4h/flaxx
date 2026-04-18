# `flaxx generate`

Scaffold a new Flux app — namespace, Kustomization, and any source resources (HelmRepository / HelmRelease / GitRepository) — in one command. Also updates the parent `kustomization.yaml` in the cluster dir so the new app is picked up.

## Synopsis

```text
flaxx generate <cluster> <app> -t <type> [flags]
```

Both positional args are required. `<cluster>` is the cluster name (also used to resolve path templates like `clusters/{{.Cluster}}`); `<app>` is the app name (also used for namespace, file prefixes, and `metadata.name` on generated resources).

## Flags

| Flag                | Default               | Description                                                                                                    |
| ------------------- | --------------------- | -------------------------------------------------------------------------------------------------------------- |
| `-t`, `--type`      | _(required)_          | Deploy type: `core`, `core-helm`, `ext-git`, `ext-helm`, `ext-oci`. See [deploy-types.md](../deploy-types.md). |
| `-n`, `--namespace` | _app name_            | Override the namespace used on generated resources.                                                            |
| `-e`, `--extra`     | _none_                | Attach one or more extras (repeatable). See [extras.md](../extras.md).                                         |
| `--set`             | _none_                | Override extra variables: `--set key=value`, repeatable.                                                       |
| `--git-url`         | —                     | Git repository URL (required for `ext-git`).                                                                   |
| `--git-branch`      | `main`                | Branch the GitRepository should track.                                                                         |
| `--git-path`        | `./deploy/production` | Path inside the external Git repository containing manifests.                                                  |
| `--git-secret`      | `git-repo-secret`     | Name of a `flux-system` Secret with Git auth.                                                                  |
| `--helm-url`        | —                     | Helm repository URL (required for `core-helm`, `ext-helm`, `ext-oci`). Must start with `oci://` for `ext-oci`. |
| `--helm-chart`      | _app name_            | Helm chart name (when it differs from the app name).                                                           |
| `--helm-version`    | _unpinned_            | Pin the HelmRelease to a specific chart version.                                                               |
| `--workload-kind`   | _none_                | With `-t core` only: emit a workload manifest (`deployment`, `statefulset`, or `daemonset`).                   |
| `--dry-run`         | `false`               | Print what would be written instead of touching the filesystem.                                                |

Global: `--config <path>` overrides config file auto-detection.

## What gets generated

For every type, flaxx creates the namespace directory (`namespace.yaml` + `kustomization.yaml`) and one Flux Kustomization in the cluster dir, then updates the cluster `kustomization.yaml`. Additional files depend on the type:

| Type        | Extra file(s) in cluster dir                         |
| ----------- | ---------------------------------------------------- |
| `core`      | (none)                                               |
| `core-helm` | `<app>-helm.yml` (HelmRepository + HelmRelease)      |
| `ext-git`   | `<app>-git.yml` (GitRepository) + dual Kustomization |
| `ext-helm`  | `<app>-helm.yml`                                     |
| `ext-oci`   | `<app>-helm.yml` (HelmRepository has `type: oci`)    |

See [deploy-types.md](../deploy-types.md) for the full file-tree preview of each type.

## Examples

### Minimal core app

```bash
flaxx generate production myapp -t core
```

### Core app with a Deployment stub

```bash
flaxx generate production myapp -t core --workload-kind deployment
```

Produces `clusters/production-namespaces/myapp/myapp-deployment.yaml` — a minimal `apps/v1` Deployment with one placeholder container — and auto-adds it to the namespace `kustomization.yaml`.

### HTTP Helm chart

```bash
flaxx generate production podinfo -t ext-helm \
  --helm-url https://stefanprodan.github.io/podinfo \
  --helm-version 6.5.4
```

### OCI Helm chart

```bash
flaxx generate production myapp -t ext-oci \
  --helm-url oci://ghcr.io/example/charts \
  --helm-chart myapp \
  --helm-version 1.2.3
```

### External Git repository

```bash
flaxx generate production myapp -t ext-git \
  --git-url https://github.com/org/myapp.git \
  --git-branch main \
  --git-path ./deploy/production
```

### With extras

```bash
flaxx generate production myapp -t core-helm \
  --helm-url https://charts.example.com \
  -e vso \
  --set vault_mount=custom-mount

# Multiple extras
flaxx generate production myapp -t core -e vso -e ingress
```

### Override the namespace

```bash
flaxx generate production myapp -t core -n custom-namespace
```

All generated manifests use `custom-namespace` for `metadata.namespace`. The namespace directory remains `<namespaces_dir>/myapp/` (directory name tracks the app name, not the namespace).

### Preview without writing

```bash
flaxx generate production myapp -t core-helm \
  --helm-url https://charts.example.com \
  --dry-run
```

Prints each generated file with a header and its intended path; nothing is written, nothing is mutated.

## Workload-kind details

`--workload-kind` is only accepted when `-t core`. It emits a minimal workload manifest into the namespaces dir using the `<app>-<kind>.yaml` naming convention:

| `--workload-kind` value | File written             | Key spec fields                                          |
| ----------------------- | ------------------------ | -------------------------------------------------------- |
| `deployment`            | `<app>-deployment.yaml`  | `replicas: 1`, `selector`, `template`                    |
| `statefulset`           | `<app>-statefulset.yaml` | above + `serviceName: <app>`, `volumeClaimTemplates: []` |
| `daemonset`             | `<app>-daemonset.yaml`   | `selector`, `template` (no `replicas`)                   |

All three use `apiVersion: apps/v1` with `app.kubernetes.io/name: <app>` labels and a single placeholder `nginx:latest` container you're expected to edit.

To migrate an existing workload between kinds later, use [`flaxx switch`](./switch.md).

## Gotchas

- **`clusters/<cluster>` must already exist** (for the flat layout) or flaxx needs permission to create the path. If you see `directory already exists: …`, you probably tried to generate an app whose namespace dir already has content — flaxx refuses to overwrite by default.
- **`--workload-kind` with anything other than `-t core` errors out.** It's a `core`-only flag because the other types deploy via Helm / external Git where the chart / repository decides the kind.
- **`--helm-url` for `ext-oci` must start with `oci://`.** You'll get `--helm-url must start with oci:// for type ext-oci` otherwise.
- **Parent `kustomization.yaml` is updated in place.** flaxx appends new resource lines rather than rewriting the whole file, so custom comments you added survive. If the file doesn't exist it is created.

## See also

- [deploy-types.md](../deploy-types.md) — type-by-type file tree preview
- [switch.md](./switch.md) — change workload kind after scaffolding
- [add.md](./add.md) — attach extras to an existing app without re-scaffolding
- [configuration.md](../configuration.md) — path and naming templating
