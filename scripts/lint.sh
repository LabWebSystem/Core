#!/usr/bin/env bash
set -Eeuo pipefail

readonly ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "$ROOT/scripts/timing.sh"

check_shell() {
  local file

  for file in "$ROOT"/scripts/*.sh; do
    bash -n "$file"
  done
}

check_go_format() {
  local diff

  diff="$(gofmt -d "$ROOT"/cmd/lwsctl/*.go "$ROOT"/backend/*.go "$ROOT"/backend/cmd/lws-backend/*.go)"

  if [[ -n "$diff" ]]; then
    printf '%s\n' "$diff"
    printf 'gofmtが必要なファイルがあります\n' >&2
    return 1
  fi
}

main() {
  trap timing_report_on_exit EXIT

  timing_begin "shell syntax"
  check_shell
  timing_end

  timing_begin "gofmt check"
  check_go_format
  timing_end

  timing_begin "go vet"
  (
    cd "$ROOT"
    go vet ./...
  )
  timing_end

  printf '静的検査: 成功\n'
}

main "$@"
