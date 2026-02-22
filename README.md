# br

A gh-like CLI for Bitrise written in Go.

## Features

- Auto-generated tag-based commands like `br addons ...`
- Generic REST execution via `br api` (works for unsupported/new endpoints too)
- Token login via `br auth login --with-token` (supports argument and stdin)
- Built-in command and option help via `help` / `--help`

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
