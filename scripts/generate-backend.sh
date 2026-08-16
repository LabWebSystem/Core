#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."
go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.8.0 \
  -config backend/oapi-codegen.yaml \
  backend/openapi.yaml
