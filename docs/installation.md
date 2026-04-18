# Installation

flaxx is a single static Go binary. Pick whichever install method fits your workflow.

## Nix flake

```bash
nix profile install github:xx4h/flaxx
```

Or consume it as a flake input:

```nix
{
  inputs.flaxx.url = "github:xx4h/flaxx";
  # ...
  outputs = { self, nixpkgs, flaxx, ... }: {
    # ... expose flaxx.packages.${system}.default
  };
}
```

The repository itself ships a development shell (`nix develop`) with Go, `golangci-lint`, `goreleaser`, `task`, `editorconfig-checker`, and `prettier` — useful if you want to hack on flaxx.

## Homebrew

```bash
brew tap xx4h/flaxx https://github.com/xx4h/flaxx
brew install xx4h/flaxx/flaxx
```

The formula installs the binary and generates shell completions for Bash, Zsh, and fish automatically on install.

## `go install`

```bash
go install github.com/xx4h/flaxx@latest
```

Installs the latest tagged release into `$GOPATH/bin` (or `$GOBIN`). Fastest option if you already have a Go toolchain.

## From source

```bash
git clone https://github.com/xx4h/flaxx.git
cd flaxx
task build
```

Produces `./bin/flaxx`. Copy it to somewhere on your `$PATH` (e.g. `~/.local/bin/`) or use `task install` (installs to `/usr/local/bin`) / `task local-install` (installs to `~/.local/bin`).

## Pre-built archives

Release artifacts for Linux, macOS, Windows, and FreeBSD are published on each tag — download from [github.com/xx4h/flaxx/releases](https://github.com/xx4h/flaxx/releases). Each archive contains the `flaxx` binary and shell-completion scripts.

## Shell completions

When installed via Homebrew or from a release archive, completions are wired up for you. For manual installs, generate them from the binary itself:

```bash
# bash
flaxx completion bash > /etc/bash_completion.d/flaxx
# zsh
flaxx completion zsh > "${fpath[1]}/_flaxx"
# fish
flaxx completion fish > ~/.config/fish/completions/flaxx.fish
```

Completions cover subcommands, flags, cluster/app positional args (by scanning the current repository), and dynamic values like available Helm chart versions for `--helm-version`.

## Verify the install

```bash
flaxx version
```

Prints version, commit, and build date baked in by `goreleaser` / `task build`.

## Uninstall

- Nix: `nix profile remove flaxx`
- Homebrew: `brew uninstall flaxx && brew untap xx4h/flaxx`
- `go install` / manual: `rm $(which flaxx)`
