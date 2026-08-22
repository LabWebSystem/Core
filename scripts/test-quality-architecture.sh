#!/usr/bin/env bash
set -Eeuo pipefail

readonly ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

require_match() {
  local pattern="$1"
  local file="$2"

  if ! grep -Eq -- "$pattern" "$file"; then
    printf '品質ゲート構造検査: %s に必要な定義がありません: %s\n' "$file" "$pattern" >&2
    return 1
  fi
}

require_match_count() {
  local pattern="$1"
  local expected="$2"
  local file="$3"
  local actual

  actual="$(grep -Ec -- "$pattern" "$file" || true)"
  if [[ "$actual" != "$expected" ]]; then
    printf '品質ゲート構造検査: %s の一致数が不正です（期待: %s、実際: %s）\n' "$file" "$expected" "$actual" >&2
    return 1
  fi
}

reject_match() {
  local pattern="$1"
  shift

  if grep -REn --exclude-dir=.venv -- "$pattern" "$@"; then
    printf '品質ゲート構造検査: 直接実行してはいけないコマンドがあります\n' >&2
    return 1
  fi
}

check_mise_tasks() {
  local mise_file="$ROOT/mise.toml"

  require_match 'architecture' "$ROOT/scripts/test.sh"
  require_match '^\[tasks\.test-sdk\]$' "$mise_file"
  require_match '^\[tasks\.test-dashboard\]$' "$mise_file"
  require_match '^\[tasks\.verify-quality\]$' "$mise_file"
  require_match '^\[tasks\.verify-release\]$' "$mise_file"
  require_match '^\[tasks\.verify\]$' "$mise_file"
}

check_workflows() {
  local ci="$ROOT/.github/workflows/ci.yml"
  local lws_release="$ROOT/.github/workflows/release-lws.yml"
  local sdk_release="$ROOT/.github/workflows/release-sdk.yml"
  local direct_runner='^[[:space:]]*-[[:space:]]*run:.*(go test|npm( run)? (test|build)|robot|scripts/test\.sh)'

  require_match 'mise run verify$' "$ci"
  require_match_count '^[[:space:]]*-[[:space:]]*run:' 1 "$ci"
  require_match 'mise run verify-release$' "$lws_release"
  require_match 'mise run test-sdk$' "$sdk_release"
  reject_match "$direct_runner" "$ci" "$lws_release" "$sdk_release"
}

check_qa_boundary() {
  local direct_runner='go test|npm( run)? test|scripts/test\.sh'

  reject_match "$direct_runner" "$ROOT/qa"
}

main() {
  check_mise_tasks
  check_workflows
  check_qa_boundary
  printf '品質ゲート構造検査: 成功\n'
}

main "$@"
