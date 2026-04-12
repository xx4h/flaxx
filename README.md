# flaxx

Generic scaffolding tool for FluxCD GitOps repositories. Generates the boilerplate YAML files needed to deploy a new app — namespace, Kustomization, HelmRelease, GitRepository, and more — from a single command.

## Installation

### Nix Flake

```bash
nix profile install github:xx4h/flaxx
```

Or add as a flake input:

```nix
{
  inputs.flaxx.url = "github:xx4h/flaxx";
}
```

### Go

```bash
go install github.com/xx4h/flaxx@latest
```

### From Source

```bash
git clone https://github.com/xx4h/flaxx.git
cd flaxx
go build -o flaxx .
```

## Quick Start

```bash
# Scaffold a Helm-based app
flaxx generate myapp k8s -t core-helm --helm-url https://charts.example.com

# Scaffold an app from an external Git repo
flaxx generate myapp k8s -t ext-git --git-url https://github.com/org/myapp.git

# Preview without writing files
flaxx generate myapp k8s -t core --dry-run
```

## Deployment Types

| Type | Description |
|------|-------------|
| `core` | Kustomization pointing to local namespace resources |
| `core-helm` | Like `core`, plus a HelmRepository and HelmRelease |
| `ext-git` | Dual Kustomization: local namespace setup + external GitRepository for app manifests |
| `ext-helm` | Like `core-helm`, but the Helm chart is pulled from an external registry |
| `ext-oci` | Like `ext-helm`, but from an OCI-compatible container registry |

### Generated Files

Each type generates files in two directories:

**Cluster directory** (`clusters/<cluster>/<app>/`):
- `<app>-kustomization.yaml` — Flux Kustomization (all types)
- `<app>-helm.yml` — HelmRepository + HelmRelease (`core-helm`, `ext-helm`, `ext-oci`)
- `<app>-git.yml` — GitRepository source (`ext-git`)

**Namespaces directory** (`clusters/<cluster>-namespaces/<app>/`):
- `namespace.yaml` — Kubernetes Namespace
- `kustomization.yaml` — Kustomize resource list

## Configuration

Place a `.flaxx.yaml` in your flux repo root to customize defaults, paths, and naming conventions:

```yaml
defaults:
  interval: 2m
  timeout: 1m
  prune: false

paths:
  cluster_dir: "clusters/{{.Cluster}}"
  namespaces_dir: "clusters/{{.Cluster}}-namespaces"

naming:
  kustomization: "{{.App}}-kustomization.yaml"
  helm: "{{.App}}-helm.yml"
  git: "{{.App}}-git.yml"
  namespace: "namespace.yaml"
  ns_kustomization: "kustomization.yaml"

templates_dir: ".flaxx/templates"
```

All fields are optional — sensible defaults are used when omitted. The config file itself is optional too.

## Custom Templates (Extras)

Extras let you define reusable template sets for things like Vault Secret Operator, cert-manager, or any other recurring pattern. They live in your flux repo under the configured `templates_dir`.

### Creating an Extra

Create a directory with a `_meta.yaml` and one or more template files:

```
.flaxx/templates/vso/
  _meta.yaml
  serviceaccount.yaml
  vso-config.yaml
```

**`_meta.yaml`** declares the extra and its variables:

```yaml
name: vso
description: Vault Secret Operator auth setup
target: namespaces
variables:
  vault_mount:
    description: Vault auth mount path
    default: "{{.Cluster}}-auth-mount"
  vault_role:
    description: Vault role name
    default: "{{.Cluster}}-{{.App}}-role"
```

**Template files** use Go `text/template` syntax with these built-in variables:
- `{{.App}}` — app name
- `{{.Cluster}}` — cluster name
- `{{.Namespace}}` — namespace (defaults to app name)

Plus any variables defined in `_meta.yaml`.

The `target` field controls where files are placed:
- `namespaces` — files go into the namespaces directory (default)
- `cluster` — files go into the cluster directory

Extra template files are automatically added to the namespace-level `kustomization.yaml` resources list.

### Using Extras

```bash
# Enable an extra
flaxx generate myapp k8s -t core-helm --helm-url https://charts.example.com -e vso

# Override extra variables
flaxx generate myapp k8s -t core -e vso --set vault_mount=custom-mount

# Multiple extras
flaxx generate myapp k8s -t core -e vso -e cert-manager
```

## CLI Reference

```
flaxx generate <app> <cluster> --type <type> [flags]

Flags:
      --dry-run               print output without writing files
  -e, --extra strings         enable extras by name (repeatable)
      --git-branch string     Git branch (default "main")
      --git-path string       path in external Git repo (default "./deploy/production")
      --git-secret string     secret name for Git auth (default "git-repo-secret")
      --git-url string        Git repository URL (required for ext-git)
      --helm-chart string     Helm chart name (default: app name)
      --helm-url string       Helm repository URL (required for helm types)
      --helm-version string   Helm chart version
      --namespace string      override namespace (default: app name)
      --set strings           override template variables (key=value, repeatable)
  -t, --type string           deployment type: core, core-helm, ext-git, ext-helm, ext-oci (required)

Global Flags:
      --config string   config file (default: auto-detect .flaxx.yaml)
```

## Development

```bash
# Run tests
go test ./...

# Build
go build -o flaxx .

# Nix dev shell
nix develop
```

## License

MIT
