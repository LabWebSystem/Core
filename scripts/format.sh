#!/usr/bin/env bash
set -Eeuo pipefail

readonly ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
readonly MODE="${1:-check}"

go_files=(
  "$ROOT"/cmd/lwsctl/*.go
  "$ROOT"/backend/*.go
  "$ROOT"/backend/cmd/lws-backend/*.go
)

format_go() {
  if [[ "$MODE" == write ]]; then
    gofmt -w "${go_files[@]}"
    return
  fi

  local diff
  diff="$(gofmt -d "${go_files[@]}")"
  if [[ -n "$diff" ]]; then
    printf '%s\n' "$diff"
    printf 'gofmtが必要なファイルがあります。mise run formatを実行してください\n' >&2
    return 1
  fi
}

format_dashboard() {
  if [[ "$MODE" == write ]]; then
    npm --prefix "$ROOT/dashboard" run format
    return
  fi

  npm --prefix "$ROOT/dashboard" run format:check
}

case "$MODE" in
  check | write)
    format_go
    format_dashboard
    ;;
  *)
    printf '使い方: scripts/format.sh [check|write]\n' >&2
    exit 2
    ;;
esac

printf 'フォーマット検査: 成功\n'
