# End-to-end walkthrough

This walkthrough takes you from an empty Git repository to a Flux-managed cluster with multiple apps and exercises every flaxx feature along the way. Every command is copy-pasteable; the file trees and output shown after each step are captured verbatim from a real run.

Rough outline:

1. [Prerequisites](#prerequisites)
2. [Bootstrap Flux](#1-bootstrap-flux)
3. [See what flaxx sees](#2-see-what-flaxx-sees)
4. [Scaffold a Helm app](#3-scaffold-a-helm-app-ext-helm)
5. [Scaffold a raw-manifest app with a workload stub](#4-scaffold-a-raw-manifest-app-with-a-workload-stub)
6. [Switch the workload kind](#5-switch-the-workload-kind)
7. [Layer on an extra](#6-layer-on-an-extra-ingress)
8. [Check for updates](#7-check-for-updates)
9. [Apply an update](#8-apply-an-update)
10. [Inspect the finished repository](#9-inspect-the-finished-repository)
11. [Multi-cluster notes](#10-multi-cluster-notes)
12. [Adopting an existing app](#11-adopting-an-existing-app)

## Prerequisites

- A Kubernetes cluster with `kubectl` access (kind, k3s, Talos, GKE, EKS — anything).
- The [`flux` CLI](https://fluxcd.io/flux/installation/) on your `$PATH` for the bootstrap step.
- A GitHub account and a [personal access token](https://github.com/settings/tokens?type=beta) with `repo` scope (Flux supports GitLab / Bitbucket / plain Git too — adjust the bootstrap command accordingly).
- `flaxx` on your `$PATH` — see [installation.md](./installation.md).

For a local run without a cluster, `kind create cluster` is enough. For the bootstrap step you still need a Git host; a private repository on github.com is fine.

## 1. Bootstrap Flux

Create a fresh repository locally, then hand it to `flux bootstrap`. This creates the `flux-system` directory, pushes its controllers' manifests to your repository, and installs Flux into the cluster.

```bash
export GITHUB_TOKEN=<your-pat>

mkdir flux-demo && cd flux-demo
git init -b main

flux bootstrap github \
  --owner=<your-github-user> \
  --repository=flux-demo \
  --path=./clusters/home \
  --branch=main \
  --personal
```

After bootstrap, the repository looks roughly like:

```text
flux-demo/
└── clusters/
    └── home/
        └── flux-system/
            ├── gotk-components.yaml
            ├── gotk-sync.yaml
            └── kustomization.yaml
```

From this point on, every commit pushed to `main` inside `clusters/home/` is reconciled by Flux into the cluster.

## 2. See what flaxx sees

```bash
flaxx inspect
```

```text
Configuration
╭───────────────────────────────────────────────────╮
│ Config file:     (none, using defaults)           │
│ Cluster dir:     clusters/{{.Cluster}}            │
│ Namespaces dir:  clusters/{{.Cluster}}-namespaces │
│ Layout:          flat                             │
│ Templates dir:   .flaxx/templates                 │
╰───────────────────────────────────────────────────╯

Cluster: home
╭────────────────────────────────────────────────────╮
│ Paths:     clusters/home, clusters/home-namespaces │
│ Layout:    empty                                   │
│ Apps:      0                                       │
╰────────────────────────────────────────────────────╯
```

No `.flaxx.yaml` yet — flaxx is using defaults, which match the layout we want. For an already-populated repository you could run `flaxx config init` here; for a fresh bootstrapped repository there's nothing to detect, so we skip it. See [commands/config.md](./commands/config.md) if you're adopting an existing repository.

## 3. Scaffold a Helm app (ext-helm)

Install podinfo — a small, well-known demo chart.

```bash
flaxx generate home podinfo -t ext-helm \
  --helm-url https://stefanprodan.github.io/podinfo \
  --helm-version 6.5.4
```

```text
Created files:
  clusters/home-namespaces/podinfo/namespace.yaml
  clusters/home-namespaces/podinfo/kustomization.yaml
  clusters/home/podinfo-kustomization.yaml
  clusters/home/podinfo-helm.yml
```

File tree:

```text
clusters/home/
├── flux-system/                (unchanged from bootstrap)
├── kustomization.yaml          (← auto-updated with new resources)
├── podinfo-kustomization.yaml  (new: Flux Kustomization)
└── podinfo-helm.yml            (new: HelmRepository + HelmRelease)

clusters/home-namespaces/podinfo/
├── kustomization.yaml          (resources: [namespace.yaml])
└── namespace.yaml
```

The Flux Kustomization file:

```yaml
---
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: podinfo
  namespace: flux-system
spec:
  interval: 2m
  targetNamespace: podinfo
  path: ./clusters/home-namespaces/podinfo
  prune: false
  sourceRef:
    kind: GitRepository
    name: flux-system
  timeout: 1m
```

The Helm file (trimmed):

```yaml
---
apiVersion: source.toolkit.fluxcd.io/v1beta2
kind: HelmRepository
metadata:
  name: podinfo
  namespace: podinfo
spec:
  interval: 0h10m0s
  url: https://stefanprodan.github.io/podinfo
---
apiVersion: helm.toolkit.fluxcd.io/v2beta1
kind: HelmRelease
metadata:
  name: podinfo
  namespace: podinfo
spec:
  chart:
    spec:
      chart: podinfo
      version: "6.5.4"
      sourceRef:
        kind: HelmRepository
        name: podinfo
        namespace: podinfo
  interval: 0h10m0s
  values: {}
```

Before committing, render what Flux is about to install — `flaxx show` pulls the chart and templates it client-side, using the version and values declared in the HelmRelease above:

```bash
flaxx show home podinfo | head -30
```

```text
---
# Source: podinfo/templates/service.yaml
apiVersion: v1
kind: Service
metadata:
  name: podinfo
  labels:
    helm.sh/chart: podinfo-6.5.4
    app.kubernetes.io/name: podinfo
…
```

Useful when you want to verify a chart's defaults, preview the effect of a `--set` override before committing it, or diff against the live cluster. See [commands/show.md](./commands/show.md) for the full flag list.

Commit and push:

```bash
git add clusters/
git commit -m "feat: add podinfo"
git push

# Ask Flux to reconcile immediately instead of waiting for the next tick:
flux reconcile kustomization flux-system --with-source
```

After reconciliation:

```bash
kubectl -n podinfo get pods
# NAME                       READY   STATUS    RESTARTS   AGE
# podinfo-6f8f59b8d8-x7g42   1/1     Running   0          30s
```

## 4. Scaffold a raw-manifest app with a workload stub

Now let's add an app with a hand-written Deployment rather than a Helm chart — `echo`. Pass `--workload-kind deployment` and flaxx writes a stub `Deployment` manifest alongside the usual namespace setup.

```bash
flaxx generate home echo -t core --workload-kind deployment
```

```text
Created files:
  clusters/home-namespaces/echo/echo-deployment.yaml
  clusters/home-namespaces/echo/namespace.yaml
  clusters/home-namespaces/echo/kustomization.yaml
  clusters/home/echo-kustomization.yaml
```

The emitted Deployment:

```yaml
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: echo
  namespace: echo
spec:
  replicas: 1
  selector:
    matchLabels:
      app.kubernetes.io/name: echo
  template:
    metadata:
      labels:
        app.kubernetes.io/name: echo
    spec:
      containers:
        - name: echo
          image: nginx:latest
```

And the namespace `kustomization.yaml` is auto-updated to include it:

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

resources:
  - namespace.yaml
  - echo-deployment.yaml
```

You are expected to edit `echo-deployment.yaml` to replace the `nginx:latest` placeholder with the real image, add ports, env vars, etc. For this walkthrough we'll leave it as-is.

See [commands/generate.md#workload-kind](./commands/generate.md#workload-kind) for the full list of kind values and what each template contains.

## 5. Switch the workload kind

Suppose we decide `echo` should actually run as a StatefulSet with a headless service. Rather than hand-edit the file, migrate in place:

```bash
flaxx switch home echo --kind statefulset --service-name echo-headless --dry-run
```

Dry-run output:

```yaml
--- clusters/home-namespaces/echo/echo-statefulset.yaml (updated) ---
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: echo
  namespace: echo
spec:
  replicas: 1
  selector:
    matchLabels:
      app.kubernetes.io/name: echo
  template:
    metadata:
      labels:
        app.kubernetes.io/name: echo
    spec:
      containers:
        - name: echo
          image: nginx:latest
  serviceName: echo-headless
  volumeClaimTemplates: []
```

Looks good — drop the `--dry-run` to commit the change:

```bash
flaxx switch home echo --kind statefulset --service-name echo-headless
```

```text
Switched Deployment → StatefulSet
  renamed: echo-deployment.yaml → echo-statefulset.yaml
Updated files:
  clusters/home-namespaces/echo/echo-statefulset.yaml
```

The file was renamed and the sibling `kustomization.yaml` was rewritten:

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

resources:
  - namespace.yaml
  - echo-statefulset.yaml
```

See the [transition matrix in commands/switch.md](./commands/switch.md#transition-matrix) for exactly which fields are added / removed per kind.

## 6. Layer on an extra (ingress)

`echo` needs an Ingress. Rather than hand-writing it, install flaxx's built-in `ingress` extra into the repository and attach it to the app.

```bash
flaxx template list
#   vso          Vault Secret Operator auth setup (VaultAuth + ServiceAccount)
#   ingress      Traefik ingress with HTTP redirect and HTTPS termination via cert-manager
#   multus       Multus macvlan NetworkAttachmentDefinition

flaxx template init ingress
# Initialized template "ingress" in .flaxx/templates/ingress
```

This created `.flaxx/templates/ingress/` containing `_meta.yaml` + `ingress.yaml`. You can (and should) edit those to match your cluster's ingress conventions.

Now attach it to `echo`:

```bash
flaxx add home echo -e ingress --set host=echo.example.com
```

```text
Files:
  clusters/home-namespaces/echo/ingress.yaml
  clusters/home-namespaces/echo/kustomization.yaml (updated)
```

The namespace `kustomization.yaml` now carries all three resources:

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

resources:
  - namespace.yaml
  - echo-statefulset.yaml
  - ingress.yaml
```

Any variable in the extra's `_meta.yaml` that wasn't overridden via `--set` uses its declared default. See [extras.md](./extras.md) for the full variable-resolution rules.

## 7. Check for updates

Time has passed, and you want to see what's available for upgrade:

```bash
flaxx check home --all
```

Sample output (truncated):

```text
Helm: podinfo
╭──────────────────────────────────────────────────────────╮
│ Chart:        podinfo                                    │
│ Repository:   https://stefanprodan.github.io/podinfo     │
│ Current:      6.5.4                                      │
│ Latest:       6.7.0                                      │
╰──────────────────────────────────────────────────────────╯
  3 update(s) available:
  6.5.5
  6.6.0
  6.7.0

Image: nginx
╭────────────────────────────────────╮
│ Image:        nginx                │
│ Container:    echo                 │
│ Current:      latest               │
│ Latest:       1.27.3               │
╰────────────────────────────────────╯
  (update listing…)
```

Filter the noise:

```bash
# Stable channel only
flaxx check home --all --stable

# Just a specific Helm chart
flaxx check home podinfo --helm podinfo
```

The cache (default 1h TTL) keeps repeated runs fast. Override or bypass with `--cache-ttl 15m` / `--no-cache`. See [commands/check.md](./commands/check.md) for the full flag list.

## 8. Apply an update

Pick one of the versions `check` surfaced and apply it:

```bash
flaxx update home podinfo --helm podinfo:6.5.5 --dry-run
```

The dry-run prints the full updated file; the only line that changes is `version: "6.5.5"`. Drop `--dry-run` and commit:

```bash
flaxx update home podinfo --helm podinfo:6.5.5
git add clusters/home/podinfo-helm.yml
git commit -m "chore: bump podinfo to 6.5.5"
git push
flux reconcile kustomization podinfo --with-source
```

`flaxx update` also works for container images in `core`-type apps:

```bash
flaxx update home echo --image echo=nginx:1.27.3
# Or target any container by name in a multi-container pod
flaxx update home multi --image sidecar=registry/sidecar:v2.0
```

## 9. Inspect the finished repository

```bash
flaxx inspect
```

```text
Configuration
╭───────────────────────────────────────────────────╮
│ Config file:     (none, using defaults)           │
│ Cluster dir:     clusters/{{.Cluster}}            │
│ Namespaces dir:  clusters/{{.Cluster}}-namespaces │
│ Layout:          flat                             │
│ Templates dir:   .flaxx/templates                 │
╰───────────────────────────────────────────────────╯

Cluster: home
╭────────────────────────────────────────────────────╮
│ Paths:     clusters/home, clusters/home-namespaces │
│ Layout:    flat                                    │
│ Apps:      2                                       │
╰────────────────────────────────────────────────────╯

  echo
    cluster:
      echo-kustomization.yaml
    namespace:
      echo-statefulset.yaml
      ingress.yaml
      kustomization.yaml
      namespace.yaml
    image: echo nginx:latest

  podinfo
    cluster:
      podinfo-kustomization.yaml
      podinfo-helm.yml
    namespace:
      kustomization.yaml
      namespace.yaml
    helm: podinfo 6.5.4 (https://stefanprodan.github.io/podinfo)
```

And the full on-disk layout:

```text
flux-demo/
├── .flaxx/
│   └── templates/
│       └── ingress/
│           ├── _meta.yaml
│           └── ingress.yaml
├── clusters/
│   ├── home/
│   │   ├── flux-system/…              (from bootstrap)
│   │   ├── echo-kustomization.yaml
│   │   ├── kustomization.yaml         (resources: [podinfo-*, echo-*])
│   │   ├── podinfo-helm.yml
│   │   └── podinfo-kustomization.yaml
│   └── home-namespaces/
│       ├── echo/
│       │   ├── echo-statefulset.yaml
│       │   ├── ingress.yaml
│       │   ├── kustomization.yaml
│       │   └── namespace.yaml
│       └── podinfo/
│           ├── kustomization.yaml
│           └── namespace.yaml
└── README.md
```

Two apps, each living in its own namespace, one managed via Helm and one via raw manifests — all reconciled by Flux from a single Git repository.

## 10. Multi-cluster notes

Nothing in the walkthrough is specific to a single cluster. To manage a second cluster — say `prod` — from the same repository:

```bash
# A second flux bootstrap, pointing at a different path:
flux bootstrap github \
  --owner=<user> --repository=flux-demo \
  --path=./clusters/prod --branch=main --personal

# Then use flaxx with cluster=prod:
flaxx generate prod podinfo -t ext-helm \
  --helm-url https://stefanprodan.github.io/podinfo \
  --helm-version 6.5.4

flaxx check prod --all
```

Directory templates (`clusters/{{.Cluster}}`, `clusters/{{.Cluster}}-namespaces`) keep each cluster's files isolated. Edit [`.flaxx.yaml`](./configuration.md) to change the templates if your repository uses a different convention (e.g. the Flux-official `clusters/<cluster>/apps/` + `apps/<app>/` layout).

## 11. Adopting an existing app

Existing clusters often already run apps that were installed out-of-band — `helm install` from a prior workflow, `kubectl apply` for a one-off, or resources left over from an earlier tool. `flaxx import` reads the live state and writes a matching flaxx-shaped app folder so those apps can join the GitOps loop alongside everything else.

```bash
# Helm-installed app (detected automatically):
flaxx import home grafana --namespace monitoring

# Raw manifests — everything in the namespace:
flaxx import home demo-app
```

For the Helm case `import` produces a `HelmRelease` + `HelmRepository`, identical to what `generate --type ext-helm` would have scaffolded. For raw apps it pulls every user-facing namespaced resource, sanitizes runtime-only fields (`status`, `managedFields`, cluster-defaulted networking, …), and wraps the result in a core-type Flux Kustomization.

Secrets are skipped by default; re-run with `--include-secrets` when you're ready to move them into an encrypted store. See [commands/import.md](./commands/import.md) for the full list of flags, filters, and sanitization rules.

## Cleanup

For a disposable test run:

```bash
flux uninstall --silent       # removes controllers from the cluster
kind delete cluster           # if you used kind
git push --delete origin main # if the repo was created just for this walkthrough
```

## Where to go next

- [deploy-types.md](./deploy-types.md) — when to pick which `-t` value.
- [extras.md](./extras.md) — author your own reusable templates.
- [commands/](./commands/) — one page per subcommand with every flag documented.
