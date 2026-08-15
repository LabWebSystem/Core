#!/usr/bin/env bash
set -Eeuo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
for file in "$ROOT"/scripts/*.sh "$ROOT"/scripts/lwsctl; do bash -n "$file"; done
printf 'lint: ok\n'
