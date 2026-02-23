# br

A gh-like CLI for Bitrise written in Go.

## Features

- Auto-generated tag-based commands like `br addons ...`
- Generic REST execution via `br api` (works for unsupported/new endpoints too)
- Token login via `br auth login --with-token` (supports argument and stdin)
- Built-in command and option help via `help` / `--help`

## Installation

### Mise (Recommended)

```bash
mise use github:shimastripe/br@latest
```

### Binary

Prebuilt binaries are available in the GitHub [Release Notes](https://github.com/shimastripe/br/releases).

## Main Commands

```bash
br auth login --with-token <TOKEN>
br auth login --with-token < token.txt
br auth status
br auth logout

br api /apps/{app-slug}/builds -X GET
br api /apps/{app-slug}/builds -X POST -f branch=main
br api /apps/{app-slug}/builds --paginate --slurp

br addons list
br builds trigger --app-slug <app-slug> -f branch=main

# GET commands default to table output
br addons list

# switch back to raw JSON output
br addons list --format json

# select only specific fields in JSON output
br addons list --format json --fields id,title

# show available fields in help
br addons list --help

# format selected JSON using a Go template
br addons list --format json --fields id,title --template '{{range .}}{{.id}} {{.title}}{{"\n"}}{{end}}'
```

## Shell Completion

Generate shell completion scripts with `br completion <shell>`.

### bash

```bash
echo 'eval "$(br completion bash)"' >> ~/.bashrc
source ~/.bashrc
```

### zsh

```bash
mkdir -p ~/.zsh/completions
br completion zsh > ~/.zsh/completions/_br
echo 'fpath=(~/.zsh/completions $fpath)' >> ~/.zshrc
echo 'autoload -U compinit && compinit -i' >> ~/.zshrc
source ~/.zshrc
```

If you use Homebrew on Apple Silicon, you can also install directly to:

```bash
br completion zsh > /opt/homebrew/share/zsh/site-functions/_br
```

### fish

```bash
mkdir -p ~/.config/fish/completions
br completion fish > ~/.config/fish/completions/br.fish
```

### powershell

```powershell
br completion powershell | Out-String | Invoke-Expression
```

## Credential Storage

- Default: save to OS keychain
- `--insecure-storage`: save plaintext token to `~/.config/br/hosts.yml`
- If `BITRISE_TOKEN` is set, it takes highest priority

## API Spec and Generation

The Bitrise Swagger spec is tracked as a fixed file, and commands are generated from it.

- Spec: `spec/bitrise-swagger.json`
- Generator: `internal/gen/openapi_gen.go`
- Generated output: `internal/cli/generated/commands_gen.go`

```bash
make generate
make check-generated
```

## Update Spec

```bash
make update-spec
```

`update-spec` downloads the latest Bitrise Swagger file and regenerates commands.

## Tests

```bash
make test
```
