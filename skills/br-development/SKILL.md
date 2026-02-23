---
name: br-development
description: Development mode for implementing and maintaining a gh-like Bitrise CLI in Go using Cobra, including token authentication (`auth login/status/logout`), generic REST execution (`api` with gh-style flags), and OpenAPI-driven tag command generation from Bitrise Swagger while excluding deprecated endpoints.
---

# br Development

Use this skill to keep the CLI architecture consistent while adding or modifying Bitrise commands.

## Core Workflow

1. Confirm current contracts before changing behavior.
2. If API surface changed, update `spec/bitrise-swagger.json` and regenerate commands.
3. Implement runtime changes in `auth`, `api`, or HTTP/common layers.
4. Re-run generation and tests.
5. Verify command help and binary build.

## Contracts to Preserve

- Keep root command shape as `br auth`, `br api`, and generated tag commands.
- Keep default host as `api.bitrise.io` and prepend `/v0.1` for non-absolute endpoints.
- Keep token resolution order: `BITRISE_TOKEN` > keychain > plaintext hosts config.
- Keep `auth login` behavior:
  - Require `--with-token`.
  - Accept token via positional arg (`br auth login --with-token XXXX`) or stdin (`br auth login --with-token < token.txt`).
  - Validate token with `GET /v0.1/me` before saving.
- Keep generated commands excluding endpoints where Swagger has `deprecated: true`.
- Keep operation alias as `operationId` on generated subcommands.
- Keep generated GET command output behavior:
  - default output format is `table` (`--format table`)
  - raw JSON output is available via `--format json`
  - field projection uses `--fields a,b,c` (not `--json`)
  - `--template` requires `--fields` and `--format json`
  - `--jq` cannot be combined with `--format table`
  - help for generated GET commands should include an `AVAILABLE FIELDS` section
- Keep `api` flag semantics aligned with gh-like behavior:
  - `-f/--raw-field` and `-F/--field` auto-switch method to `POST` unless method explicitly set.
  - With `--input`, treat fields as query params and use input as body.

## File Map

- Root wiring: `cmd/br/main.go`, `internal/cli/root.go`
- Auth: `internal/cli/authcmd/auth.go`, `internal/auth/store.go`, `internal/config/hosts.go`
- Generic API: `internal/cli/apicmd/api.go`, `internal/httpclient/client.go`
- Generator: `internal/gen/openapi_gen.go`
- Generated output: `internal/cli/generated/commands_gen.go`, `internal/cli/generated/runtime.go`, `internal/cli/generated/types.go`
- Spec: `spec/bitrise-swagger.json`
- Developer tasks: `Makefile`, `tools/update_spec.sh`

## Update Procedure

When Bitrise API changes:

1. Run `make update-spec`.
2. Run `make generate`.
3. Check generated diff in `internal/cli/generated/commands_gen.go`.
4. If naming/flags/body handling need adjustment, update generator or runtime.
5. Run full verification commands.

## Verification Commands

Run from repository root:

```bash
make generate
make check-generated
make test
GOCACHE=/tmp/go-build GOMODCACHE=/tmp/gomodcache go build -o bin/br ./cmd/br
./bin/br --help
./bin/br auth login --help
./bin/br builds list --help
```

## Debugging Checklist

- Auth failures: inspect `Authorization` header source and `/v0.1/me` validation flow.
- Command missing: confirm operation is non-deprecated and tag mapping is expected.
- Endpoint mismatch: verify generated `Path` and base-path normalization.
- Pagination issues: inspect `paging.next` handling and `--slurp` behavior.

Load `references/bitrise-cli-contract.md` when you need concrete acceptance criteria and regression checks.
