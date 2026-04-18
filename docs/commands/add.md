# `flaxx add`

Attach one or more extras to an already-scaffolded app. Renders the extra's templates into the app directory and updates the namespace-level `kustomization.yaml` so the new files are picked up.

Use `add` when you want to layer more functionality onto an existing app — for example adding Vault auth or an Ingress — without re-running `generate`.

## Synopsis

```text
flaxx add <cluster> <app> -e <extra> [flags]
```

Both positional args are required; both must already exist as scaffolded by an earlier `flaxx generate`. At least one `-e <extra>` is required.

## Flags

| Flag                | Default      | Description                                                     |
| ------------------- | ------------ | --------------------------------------------------------------- |
| `-e`, `--extra`     | _(required)_ | Extra(s) to add. Repeat for multiple.                           |
| `--set`             | _none_       | Override extra variables: `--set key=value`, repeatable.        |
| `-n`, `--namespace` | _app name_   | Override namespace used in rendered templates.                  |
| `--dry-run`         | `false`      | Print what would be written instead of touching the filesystem. |

Global: `--config <path>`.

## What gets written

- Files from each extra are rendered into the target directory (namespace dir by default; cluster dir if `target: cluster`; routed per subdir if `target: split`).
- The namespace `kustomization.yaml` is updated to reference every new namespace-targeted file. Existing resource lines are preserved.

Extras are discovered from the configured `templates_dir` (default `.flaxx/templates/`). See [extras.md](../extras.md) for the `_meta.yaml` schema and variable resolution.

## Examples

### Add one extra

```bash
flaxx add production myapp -e vso
```

### Override variables

```bash
flaxx add production myapp -e vso \
  --set vault_mount=custom-mount \
  --set vault_role=custom-role
```

### Add multiple extras in one call

```bash
flaxx add production myapp -e ingress -e vso --set host=myapp.example.com
```

All extras see the same `--set` values — scope your overrides using unique variable names across extras to avoid collision.

### Preview without writing

```bash
flaxx add production myapp -e ingress --set host=myapp.example.com --dry-run
```

## Gotchas

- **The app must already exist** (generated earlier). `add` does not create the namespace dir or parent kustomization. If the app isn't there you'll get a file-not-found or missing-directory error.
- **Extras with `target: cluster`** write into the cluster dir but are not auto-registered in the cluster `kustomization.yaml` — you are expected to reference them manually or have the cluster Kustomization pick them up through a resource glob. (Only namespace-targeted extras update the namespace kustomization.)
- **No conflict detection on individual files.** If an extra renders a filename that already exists in the target directory, it will be overwritten. Use `--dry-run` first if you aren't sure.

## See also

- [extras.md](../extras.md) — authoring your own extras
- [commands/template.md](./template.md) — list, init, and extract extras
- [commands/generate.md](./generate.md) — apply extras at scaffold time
