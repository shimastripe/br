---
name: br-usage
description: Use when operating Bitrise from the `br` CLI (not developing it): login, app/build/pipeline commands, output formatting with `--format` and `--fields`, and fallback requests with `br api`.
---

# Bitrise CLI Usage

Use this skill when a user asks how to run Bitrise workflows via CLI commands.

## Quick Start
```bash
br auth login --with-token <TOKEN>
br auth status
br application app-list
br builds list --app-slug <app-slug>
```

## Common Tasks

1. Authentication
- `br auth login --with-token <TOKEN>`
- `br auth login --with-token < token.txt>`
- `br auth status`
- `br auth logout`

2. App discovery
- `br application app-list`
- `br application app-show --app-slug <app-slug>`

3. Builds and pipelines
- `br builds list --app-slug <app-slug>`
- `br builds show --app-slug <app-slug> --build-slug <build-slug>`
- `br builds trigger --app-slug <app-slug> -f branch=main`
- `br pipelines list --app-slug <app-slug>`
- `br pipelines show --app-slug <app-slug> --pipeline-id <pipeline-id>`

4. Logs and details
- `br builds log --app-slug <app-slug> --build-slug <build-slug>`
- `br builds workflow-list --app-slug <app-slug>`

## Output and Filtering Rules

- Generated GET commands default to `--format table`.
- Use `--format json` when piping output to scripts.
- Use `--fields a,b,c` to select response fields.
- `--template` requires both `--format json` and `--fields`.
- `--jq` is for JSON mode; do not combine it with `--format table`.
- Check `<command> --help` for `AVAILABLE FIELDS`.

## Generic API Fallback

Use `br api` when a generated command does not cover the endpoint yet:

```bash
br api /apps/<app-slug>/builds -X GET
br api /apps/<app-slug>/builds -X POST -f branch=main
br api /apps/<app-slug>/builds --paginate --slurp
```

Key behavior:
- Relative endpoints are resolved to `https://api.bitrise.io/v0.1`.
- `-f/-F` imply `POST` unless method is explicitly set.
- With `--input`, fields are sent as query params.

## Troubleshooting

- `401 Unauthorized`: verify token with `br auth status`, then re-login.
- `unknown flag`: run `<command> --help` and use tag command-specific flags.
- Missing generated command: use `br api` for the same endpoint.
- Host mismatch: use `--hostname` on command/auth calls.

Load `references/command-recipes.md` for copy-paste command examples.
