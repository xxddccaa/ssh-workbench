#!/bin/sh
set -eu

project_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
temporary=$(mktemp)
trap 'rm -f "$temporary"' EXIT HUP INT TERM
cd "$project_root"
go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.9.0 -generate types,skip-prune -package contract -o "$temporary" api/openapi.json
if ! cmp -s "$temporary" internal/transport/httpapi/generated/api.gen.go; then
  printf '%s\n' 'Generated Go API types have drifted. Run make generate.' >&2
  diff -u internal/transport/httpapi/generated/api.gen.go "$temporary" || true
  exit 1
fi
