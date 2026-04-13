# Contributing to flaxx

## Development Setup

```bash
# Clone the repo
git clone https://github.com/xx4h/flaxx.git
cd flaxx

# Using Nix (recommended — includes Go, golangci-lint, goreleaser, task)
nix develop
# Or with direnv: `direnv allow` for automatic shell activation

# Build
task build

# See all available targets
task --list
```

## Making Changes

1. Create a feature branch from `main`
2. Make your changes
3. Add or update tests as needed
4. Run the test suite: `task test-unit`
5. Run the linter: `task test-style`
6. Commit using conventional commits (see below)
7. Open a pull request against `main`

## Commit Messages

This project uses [Conventional Commits](https://www.conventionalcommits.org/). Every commit message must follow this format:

```text
<type>[optional scope]: <description>

[optional body]

[optional footer(s)]
```

### Types

| Type       | Description                                             |
| ---------- | ------------------------------------------------------- |
| `feat`     | A new feature                                           |
| `fix`      | A bugfix                                                |
| `docs`     | Documentation only changes                              |
| `refactor` | Code change that neither fixes a bug nor adds a feature |
| `test`     | Adding or updating tests                                |
| `build`    | Changes to the build tool or dependencies               |
| `ci`       | Changes to CI configuration                             |
| `chore`    | Other changes that don't modify src or test files       |

### Scopes

Scopes are optional but encouraged. Common scopes for this project:

- `cmd` — CLI commands and flags
- `checker` — version checking logic
- `generator` — scaffolding generation
- `updater` — YAML mutation for updates
- `config` — configuration loading
- `extras` — template extras system
- `completions` — shell completions

### Examples

```text
feat(checker): add OCI registry support for version checking

fix(updater): preserve YAML quoting style when updating helm version

docs: add Homebrew install instructions to README

test(cmd): add positional arg order tests for all subcommands

build: update vendorHash in flake.nix for new dependency
```

### Rules

- Use the imperative mood in the description ("add" not "added")
- Do not capitalize the first letter of the description
- No period at the end of the description
- Keep the first line under 72 characters
- Use the body to explain _what_ and _why_, not _how_

## Code Style

- Follow standard Go conventions (`gofmt`, `go vet`)
- Keep functions focused — prefer small, testable units
- Use table-driven tests where appropriate
- No unnecessary abstractions — three similar lines beat a premature helper

## Testing

All changes should include tests. Run the full suite before submitting:

```bash
task test-unit
```

### Test Guidelines

- Internal packages: test the exported API directly
- CLI commands: test via cobra's argument parsing (see `cmd/cmd_test.go`)
- Use `t.TempDir()` for tests that need filesystem state
- Use `httptest.NewServer` / `httptest.NewTLSServer` for HTTP tests
- Reset package-level flag variables between cobra command tests (see `resetFlags()`)

## Project Structure

```text
cmd/               CLI command definitions and completions
internal/
  builtin/         Built-in extra templates
  checker/         Version checking (Helm repos, OCI registries, container images)
  config/          Configuration loading (.flaxx.yaml)
  extras/          Custom template discovery and rendering
  generator/       Scaffolding generation and add logic
  templates/       Core Go templates for Flux resources
  updater/         YAML mutation for helm versions and images
```

## Pull Requests

- Keep PRs focused on a single concern
- Update or add tests for any changed behavior
- Make sure `task test-unit` passes
- Use a conventional commit style title for the PR
