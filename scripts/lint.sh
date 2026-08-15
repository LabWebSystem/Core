#!/usr/bin/env bash
set -Eeuo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
for file in "$ROOT"/scripts/*.sh; do bash -n "$file"; done
test -z "$(gofmt -d "$ROOT"/cmd/lwsctl/main.go)"
go vet ./cmd/lwsctl
printf '静的検査: 成功\n'
