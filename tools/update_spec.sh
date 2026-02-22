#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SPEC_URL="https://api-docs.bitrise.io/docs/swagger.json"
SPEC_PATH="${ROOT_DIR}/spec/bitrise-swagger.json"

curl -fsSL "${SPEC_URL}" -o "${SPEC_PATH}"

GOCACHE="${GOCACHE:-/tmp/go-build}" \
GOMODCACHE="${GOMODCACHE:-/tmp/gomodcache}" \
  go run "${ROOT_DIR}/internal/gen/openapi_gen.go"
