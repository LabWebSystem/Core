#!/usr/bin/env bash
set -Eeuo pipefail

readonly ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

check_shell() {
  local file

  for file in "$ROOT"/scripts/*.sh; do
    bash -n "$file"
  done
}

check_go_format() {
  local diff

  diff="$(gofmt -d "$ROOT"/cmd/lwsctl/*.go)"

  if [[ -n "$diff" ]]; then
    printf '%s\n' "$diff"
    printf 'gofmtが必要なファイルがあります\n' >&2
    return 1
  fi
}

main() {
  check_shell
  check_go_format

  (
    cd "$ROOT"
    go vet ./cmd/lwsctl
  )

  printf '静的検査: 成功\n'
}

main "$@"