# `flaxx template`

Three subcommands for working with extras (reusable template sets):

- `flaxx template list` — show available built-in templates
- `flaxx template init <name>` — copy a built-in template into your repository
- `flaxx template from-app <cluster> <app> <name>` — extract an existing app into a reusable template

See [extras.md](../extras.md) for what an extra actually is and how it's rendered.

## `flaxx template list`

```bash
flaxx template list
```

Output:

```text
  vso          Vault Secret Operator auth setup (VaultAuth + ServiceAccount)
  ingress      Traefik ingress with HTTP redirect and HTTPS termination via cert-manager
  multus       Multus macvlan NetworkAttachmentDefinition
```

These are the built-ins compiled into the binary. Custom extras you authored under `templates_dir` are not listed here — use `flaxx inspect` or look under `.flaxx/templates/` directly.

## `flaxx template init <name> [name ...]`

Writes a built-in template into your configured `templates_dir` so you can edit it and track it in Git.

```bash
flaxx template init vso
# Initialized template "vso" in .flaxx/templates/vso

# Multiple at once
flaxx template init vso ingress multus
```

After init, the template is a regular user-authored extra — edit it freely, and it will be picked up by `flaxx generate -e <name>` / `flaxx add -e <name>` in the usual way.

### Init gotchas

- **Won't overwrite** an existing template of the same name: `template "vso" already exists at …`. Delete or rename the directory to re-init.
- **Unknown name** errors out with a pointer to `flaxx template list`.

## `flaxx template from-app` {#from-app}

Extract an existing app's files into a reusable template. Useful when you built an app manually (or with flaxx plus heavy manual edits) and now want to replicate the pattern across more apps.

```text
flaxx template from-app <cluster> <app> <template-name> [flags]
```

All three positional args are required.

### Flags

| Flag                  | Default | Description                                                                                                                         |
| --------------------- | ------- | ----------------------------------------------------------------------------------------------------------------------------------- |
| `--include-cluster`   | `false` | Also capture cluster-directory files (Flux Kustomization, HelmRelease, GitRepository). The resulting template uses `target: split`. |
| `-i`, `--interactive` | `false` | Confirm each detected variable before writing. Reads from stdin.                                                                    |
| `--force`             | `false` | Overwrite an existing template of the same name.                                                                                    |
| `--dry-run`           | `false` | Print what would be written; do not modify anything.                                                                                |
| `--description`       | _none_  | Description line written into `_meta.yaml`.                                                                                         |

### What it does

1. Reads every file in the app's namespace directory (and, with `--include-cluster`, cluster directory).
2. Replaces occurrences of the app name / cluster name / namespace with `{{.App}}` / `{{.Cluster}}` / `{{.Namespace}}` placeholders.
3. Detects likely parameterization targets (Helm chart name, image tag, ingress host, Git URL) and offers them as `variables:` in `_meta.yaml`.
4. Writes the result to `<templates_dir>/<template-name>/`.

With `--include-cluster`, the template is split across two subdirectories (`cluster/` and `namespaces/`), which triggers flaxx's `target: split` routing when the extra is later used.

### Examples

```bash
# Namespace-only template (simple case)
flaxx template from-app production myapp myapp-template \
  --description "Myapp flavor of production deployment"

# Full-split template including the Flux Kustomization + HelmRelease
flaxx template from-app production myapp myapp-template --include-cluster

# Interactive: review detected variables before commit
flaxx template from-app production myapp myapp-template -i

# Preview without writing
flaxx template from-app production myapp myapp-template --include-cluster --dry-run
```

### from-app gotchas

- **An existing template blocks the write** unless you pass `--force`. `--force` wipes the old directory before writing — commit before using.
- **Variable detection is heuristic.** Review the generated `_meta.yaml` — you may want to rename variables, tighten descriptions, or remove detections that aren't meaningful.
- **Rendered output is best-effort.** Some manifests contain values that look app-specific but aren't (e.g. RBAC role names that match the app name by convention). Run the template back through `flaxx generate -e <name>` in a scratch dir and diff against the original to catch surprises.

## See also

- [extras.md](../extras.md) — anatomy of an extra, the `_meta.yaml` schema, variable resolution
- [commands/add.md](./add.md) — apply an extra to an existing app
- [commands/generate.md](./generate.md) — apply an extra at scaffold time
