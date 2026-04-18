# flaxx

<p align="center">
  <img alt="GitHub stars" src="https://img.shields.io/github/stars/xx4h/flaxx">
  <img alt="GitHub forks" src="https://img.shields.io/github/forks/xx4h/flaxx">
</p>

<!-- markdownlint-disable no-empty-links -->

[![Lint Code Base](https://github.com/xx4h/flaxx/actions/workflows/linter-full.yml/badge.svg)](https://github.com/xx4h/flaxx/actions/workflows/linter-full.yml)
[![Test Code Base](https://github.com/xx4h/flaxx/actions/workflows/test-full.yml/badge.svg)](https://github.com/xx4h/flaxx/actions/workflows/test-full.yml)
[![GitHub Downloads (all assets, all releases)](https://img.shields.io/github/downloads/xx4h/flaxx/total)](https://github.com/xx4h/flaxx/releases)
[![Go Report Card](https://goreportcard.com/badge/github.com/xx4h/flaxx?)](https://goreportcard.com/report/github.com/xx4h/flaxx)
[![Number of programming languages used](https://img.shields.io/github/languages/count/xx4h/flaxx)](#)
[![Top programming languages used](https://img.shields.io/github/languages/top/xx4h/flaxx)](#)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue)](LICENSE)
[![Latest tag](https://img.shields.io/github/v/tag/xx4h/flaxx)](https://github.com/xx4h/flaxx/tags)
[![Closed issues](https://img.shields.io/github/issues-closed/xx4h/flaxx?color=success)](https://github.com/xx4h/flaxx/issues?q=is%3Aissue+is%3Aclosed)
[![Closed PRs](https://img.shields.io/github/issues-pr-closed/xx4h/flaxx?color=success)](https://github.com/xx4h/flaxx/pulls?q=is%3Apr+is%3Aclosed)
<br>

<!-- markdownlint-enable no-empty-links -->

Scaffolding and maintenance tool for FluxCD GitOps repositories. Generates the boilerplate YAML files needed to deploy a new app — namespace, Kustomization, HelmRelease, GitRepository, and more — and helps maintain them by checking for newer Helm chart and container image versions.

> Full documentation and an end-to-end walkthrough live in [`docs/`](./docs/README.md).

## Why flaxx?

Adding a new app to a Flux repository means creating the same set of files every time: a namespace, a Kustomize resource list, a Flux Kustomization, maybe a HelmRepository and HelmRelease — all wired together with the right paths and naming. It's tedious, error-prone, and the kind of thing you get wrong just often enough to waste time debugging a typo in a sourceRef.

flaxx handles the scaffolding so you don't have to. One command generates all the files, adds them to the parent kustomization, and follows the conventions your repository already uses. It also helps with ongoing maintenance: checking upstream Helm repos and container registries for newer versions, and updating them in place.

## Getting Started

### Starting from scratch

If you're setting up a new Flux repository:

```bash
mkdir my-flux-repo && cd my-flux-repo
git init

# Create your first app
flaxx generate production myapp -t core-helm --helm-url https://charts.example.com

# flaxx creates the directory structure and all required files:
#   clusters/production/myapp-kustomization.yaml
#   clusters/production/myapp-helm.yml
#   clusters/production/kustomization.yaml  (auto-managed)
#   clusters/production-namespaces/myapp/namespace.yaml
#   clusters/production-namespaces/myapp/kustomization.yaml
```

You can customize the paths and naming by creating a `.flaxx.yaml` — or just use the defaults, which follow the Flux-recommended flat layout.

### Adopting an existing repository

If you already have a Flux repository with apps deployed:

```bash
cd /path/to/your/flux-repo

# Let flaxx detect your directory structure
flaxx config init

# This scans for cluster directories, namespace directories,
# and whether you use flat files or per-app subdirectories,
# then generates a .flaxx.yaml that matches your layout.

# Verify what flaxx sees
flaxx inspect

# Now you can add new apps and they'll follow your existing conventions
flaxx generate production newapp -t core-helm --helm-url https://charts.example.com

# Check all existing apps for available updates
flaxx check production --all
```

flaxx detects common Flux layouts automatically, including `clusters/<cluster>/apps/` + `apps/` (Flux standard) and `clusters/<cluster>/` + `clusters/<cluster>-namespaces/` patterns.

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

### Homebrew

```bash
brew tap xx4h/flaxx https://github.com/xx4h/flaxx
brew install xx4h/flaxx/flaxx
```

### Go

```bash
go install github.com/xx4h/flaxx@latest
```

### From Source

```bash
git clone https://github.com/xx4h/flaxx.git
cd flaxx
task build
```

## Quick Start

```bash
# Auto-detect your repo structure and generate config
flaxx config init

# Scaffold a Helm-based app
flaxx generate production myapp -t core-helm --helm-url https://charts.example.com

# Scaffold an app from an external Git repo
flaxx generate production myapp -t ext-git --git-url https://github.com/org/myapp.git

# Preview without writing files
flaxx generate production myapp -t core --dry-run

# Check for newer chart and image versions
flaxx check production myapp

# Check all apps in a cluster
flaxx check production --all

# Update a Helm chart version
flaxx update production myapp --helm-version 2.0.0

# Inspect the repository structure
flaxx inspect
```

## Repository Layout

flaxx supports two cluster directory layouts:

### Flat layout (default, Flux-recommended)

Files go directly into the cluster directory — no per-app subdirectories:

```text
clusters/<cluster>/
  <app>-kustomization.yaml    # Flux Kustomization
  <app>-helm.yml              # HelmRepository + HelmRelease
  kustomization.yaml           # auto-managed resource list
<namespaces-dir>/<app>/
  namespace.yaml
  kustomization.yaml
```

### Subdirs layout

Enable with `cluster_subdirs: true` in `.flaxx.yaml`:

```text
clusters/<cluster>/<app>/
  <app>-kustomization.yaml
  <app>-helm.yml
<namespaces-dir>/<app>/
  namespace.yaml
  kustomization.yaml
```

## Deployment Types

| Type        | Description                                                                          |
| ----------- | ------------------------------------------------------------------------------------ |
| `core`      | Kustomization pointing to local namespace resources                                  |
| `core-helm` | Like `core`, plus a HelmRepository and HelmRelease                                   |
| `ext-git`   | Dual Kustomization: local namespace setup + external GitRepository for app manifests |
| `ext-helm`  | Like `core-helm`, but the Helm chart is pulled from an external registry             |
| `ext-oci`   | Like `ext-helm`, but from an OCI-compatible container registry                       |

## Configuration

Place a `.flaxx.yaml` in your flux repository root, or generate one automatically:

```bash
# Detect structure and generate config
flaxx config init

# Preview what config would be generated
flaxx config show
```

Example `.flaxx.yaml`:

```yaml
defaults:
  interval: 2m
  timeout: 1m
  prune: false

paths:
  cluster_dir: "clusters/{{.Cluster}}/apps"
  namespaces_dir: "apps"
  cluster_subdirs: false

naming:
  kustomization: "{{.App}}-kustomization.yaml"
  helm: "{{.App}}-helm.yml"
  git: "{{.App}}-git.yml"
  namespace: "namespace.yaml"
  ns_kustomization: "kustomization.yaml"

templates_dir: ".flaxx/templates"
```

All fields are optional — sensible defaults are used when omitted. The config file itself is optional too.

## Version Checking

flaxx can check for newer versions of Helm charts and container images:

```bash
# Check a single app
flaxx check production myapp

# Check all apps in a cluster
flaxx check production --all
```

Supports standard Helm repositories, OCI registries (with token auth and pagination), and container image tags from Deployment/StatefulSet/DaemonSet resources.

## Updating

```bash
# Bump Helm chart version
flaxx update production myapp --helm-version 2.0.0

# Update container image
flaxx update production myapp --image registry/myapp:v1.2.3

# Update a specific container in a multi-container pod
flaxx update production myapp --image sidecar=registry/sidecar:v2.0
```

Shell completions for `--helm-version` and `--image` query upstream registries for available versions.

## Custom Templates (Extras)

Extras let you define reusable template sets for things like Vault Secret Operator, cert-manager, or any other recurring pattern. They live in your flux repository under the configured `templates_dir`.

### Creating an Extra

Create a directory with a `_meta.yaml` and one or more template files:

```text
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

### Using Extras

```bash
# Enable an extra
flaxx generate production myapp -t core-helm --helm-url https://charts.example.com -e vso

# Override extra variables
flaxx generate production myapp -t core -e vso --set vault_mount=custom-mount

# Multiple extras
flaxx generate production myapp -t core -e vso -e cert-manager
```

Built-in extras (initialize with `flaxx template init <name>`):

- `vso` — Vault Secret Operator auth setup
- `ingress` — Traefik ingress with cert-manager
- `multus` — Multus macvlan NetworkAttachmentDefinition

## CLI Reference

```text
Commands:
  generate <cluster> <app>    Generate scaffolding files for a new Flux app
  add <cluster> <app>         Add extras to an existing app
  update <cluster> <app>      Update Helm version or container image
  check <cluster> [<app>]     Check for newer versions (use --all for all apps)
  inspect                     Analyze the repository structure
  config show                 Preview detected configuration
  config init                 Generate .flaxx.yaml from detected structure
  template list               List available built-in templates
  template init <name>        Initialize built-in templates into your repo
  version                     Print version information

Global Flags:
  --config string   config file (default: auto-detect .flaxx.yaml)
```

## Development

```bash
# Nix dev shell (includes Go, golangci-lint, goreleaser, task)
nix develop

# Build
task build

# Run tests
task test-unit

# Run linter
task test-style

# All checks
task all

# See all targets
task --list
```
