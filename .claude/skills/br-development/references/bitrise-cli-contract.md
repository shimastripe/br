# Bitrise CLI Contract Reference

## Command Surface

- Root commands:
  - `br auth`
  - `br api`
  - Generated tag commands from Bitrise Swagger tags
- Example mapping:
  - API path `/apps` => `br application app-list`
  - Generic fallback => `br api /apps -X GET`

## Generated Command Rules

- Source spec: `spec/bitrise-swagger.json`
- Include operations where `deprecated != true`
- Exclude deprecated operations such as:
  - `secret-upsert`
  - `secret-value-get`
- Derive subcommand name from `operationId` by removing:
  - `<tag>-`
  - `<singular(tag)>-`
- Keep `operationId` as command alias to avoid discoverability loss.
- For generated GET operations:
  - default output format is `table`
  - supported formats are `--format {table|json}`
  - field projection uses `--fields`
  - help includes `AVAILABLE FIELDS`
  - `--template` requires `--fields` and `--format json`
  - `--jq` cannot be used with `--format table`

## Auth Rules

- `br auth login --with-token XXXX`:
  - `--with-token` gates token mode.
  - Positional token `XXXX` is accepted.
- `br auth login --with-token`:
  - Read token from stdin.
- Validate with `GET /v0.1/me` before persisting token.
- Save default: keychain.
- Save fallback: plaintext config file when `--insecure-storage` is set.
- Resolve token order:
  - `BITRISE_TOKEN`
  - keychain
  - hosts config (`~/.config/br/hosts.yml` unless overridden by `XDG_CONFIG_HOME`)

## Generic API Rules

- Endpoint resolution:
  - Accept `/apps/...` and `apps/...`
  - Resolve against `https://<host>/v0.1`
- Method behavior:
  - default `GET`
  - if fields are provided and method not explicit, switch to `POST`
- Body/query behavior:
  - without `--input`, fields go to query for `GET`, otherwise JSON body
  - with `--input`, input becomes body and fields become query params
- Pagination:
  - `--paginate` follows `paging.next`
  - `--slurp` requires `--paginate`

## Regression Checklist

- Generated command count stays equal to non-deprecated operation count in spec.
- `make check-generated` has zero diff after regeneration.
- `make test` passes.
- `go build` succeeds and help output includes `auth`, `api`, and generated tag groups.
- Generated GET help output includes `AVAILABLE FIELDS`, `--fields`, and `--format`.

## High-value Integration Tests

- Login with token arg persists token after validation.
- Login via stdin behaves identically.
- Generated command (e.g. `builds list`) hits expected path and method.
- `br api -X PATCH ... -f ... -H ...` sends expected method/body/header.
- Non-2xx responses return non-zero and print error payload.
- Pagination + slurp returns aggregated JSON array.
