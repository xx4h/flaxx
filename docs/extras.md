# Extras

Extras are reusable template sets. Use them for recurring patterns like Vault auth, a cert-manager Ingress, or a Multus NetworkAttachmentDefinition — anything you'd otherwise copy/paste between apps.

They live under the configured `templates_dir` (default `.flaxx/templates/`) at the root of your Flux repository, and are referenced by name with `-e <name>` on `flaxx generate` and `flaxx add`.

## Layout

Each extra is a directory whose name is the extra's name. Inside, a `_meta.yaml` declares it plus any template files:

```text
.flaxx/templates/
├── vso/
│   ├── _meta.yaml
│   ├── serviceaccount.yaml
│   └── vso-config.yaml
└── ingress/
    ├── _meta.yaml
    └── ingress.yaml
```

Template files use Go's [`text/template`](https://pkg.go.dev/text/template) syntax.

## `_meta.yaml` schema

```yaml
name: vso # required; must match the directory name
description: Vault Secret Operator auth setup # shown in `flaxx template list` and completions
target: namespaces # one of: namespaces, cluster, split (see below)

variables:
  vault_mount:
    description: Vault auth mount path
    default: "{{.Cluster}}-auth-mount"
  vault_role:
    description: Vault role name
    default: "{{.Cluster}}-{{.App}}-role"
```

### `name` / `description`

Free text. `name` must be unique within the templates dir; it is what the user passes to `-e`.

### `target`

Controls where rendered files land:

| Value                  | Files go to                                               |
| ---------------------- | --------------------------------------------------------- |
| `namespaces` (default) | `<namespaces_dir>/<app>/`                                 |
| `cluster`              | The app's directory inside `<cluster_dir>`                |
| `split`                | Route by subdirectory (see [split extras](#split-extras)) |

Files with `target: namespaces` are automatically added to the namespace's `kustomization.yaml` `resources:` list. Files with `target: cluster` are not — they are expected to be referenced by the cluster-level kustomization or to stand alone.

### `variables`

Each entry declares one input the template expects.

- `description` — human-readable text; shown in shell completions for `--set`.
- `default` — value used if `--set <name>=<value>` is not passed. Default strings are themselves Go templates and can reference `{{.App}}`, `{{.Cluster}}`, `{{.Namespace}}`.

At render time, the user's `--set name=value` overrides take precedence over defaults.

## Template variables

Inside any template file you can reference:

| Variable                     | Value                                               |
| ---------------------------- | --------------------------------------------------- |
| `{{.App}}`                   | The app name                                        |
| `{{.Cluster}}`               | The cluster name                                    |
| `{{.Namespace}}`             | The resolved namespace (app name, or `-n` override) |
| `{{.vault_mount}}` (example) | Any variable you declared in `variables:`           |

Template errors (e.g. referencing a variable you didn't declare) surface at `generate` / `add` time with a clear error message.

## Using an extra

```bash
# Attach one extra during scaffold
flaxx generate production myapp -t core-helm \
  --helm-url https://charts.example.com \
  -e vso

# Override a variable
flaxx generate production myapp -t core -e vso --set vault_mount=custom-mount

# Multiple extras at once
flaxx generate production myapp -t core -e vso -e ingress

# Add an extra to an already-scaffolded app
flaxx add production myapp -e ingress --set host=myapp.example.com
```

`flaxx add` is covered in [commands/add.md](./commands/add.md). The two commands share the same extra rendering path, so everything in this page applies to both.

## Built-in extras

flaxx ships three extras you can install into your repository without writing them yourself:

```bash
flaxx template list
# vso          Vault Secret Operator auth setup (VaultAuth + ServiceAccount)
# ingress      Traefik ingress with HTTP redirect and HTTPS termination via cert-manager
# multus       Multus macvlan NetworkAttachmentDefinition

flaxx template init vso        # writes .flaxx/templates/vso/
flaxx template init ingress    # writes .flaxx/templates/ingress/
```

Once initialized they are just regular extras in your repository — edit them, commit them, adapt them to taste.

## Authoring a custom extra

Concrete example: a minimal cert-manager ClusterIssuer reference plus a ConfigMap with the app's display name.

**Create the directory:**

```bash
mkdir -p .flaxx/templates/welcome
```

**`.flaxx/templates/welcome/_meta.yaml`:**

```yaml
name: welcome
description: Welcome ConfigMap with a display name
target: namespaces
variables:
  display_name:
    description: Human-friendly name
    default: "{{.App}}"
  admin_email:
    description: Contact email
    default: ops@example.com
```

**`.flaxx/templates/welcome/welcome-config.yaml`:**

```yaml
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: {{.App}}-welcome
  namespace: {{.Namespace}}
data:
  display-name: {{.display_name}}
  admin: {{.admin_email}}
```

**Use it:**

```bash
flaxx add production myapp -e welcome --set display_name="My App"
```

The resulting file is written to `clusters/production-namespaces/myapp/welcome-config.yaml` and the namespace `kustomization.yaml` is updated to include it.

## Split extras

When you set `target: split`, flaxx routes files based on which subdirectory they live in inside the extra.

```text
.flaxx/templates/backup/
├── _meta.yaml            # target: split
├── cluster/              # files here land in <cluster_dir>/<app>/
│   └── backup-cronjob.yaml
└── namespaces/           # files here land in <namespaces_dir>/<app>/
    └── backup-pvc.yaml
```

This is the same layout `flaxx template from-app --include-cluster` produces — it's the right target when a single extra needs to contribute both cluster-level (Flux) resources and namespace-level (workload) resources.

## Extracting an extra from an existing app

If you already have an app you want to turn into a reusable extra:

```bash
flaxx template from-app production myapp my-template
```

This reads all the files of `myapp`, replaces occurrences of the app name / cluster / namespace with template variables, and writes `.flaxx/templates/my-template/`. Detected values (Helm chart, image tags, ingress hosts, Git URLs) become variables in `_meta.yaml`.

See [commands/template.md](./commands/template.md#from-app) for the full flag set.

## See also

- [commands/generate.md](./commands/generate.md) — `-e` / `--set` during scaffold
- [commands/add.md](./commands/add.md) — adding extras to existing apps
- [commands/template.md](./commands/template.md) — `template list`, `template init`, `template from-app`
