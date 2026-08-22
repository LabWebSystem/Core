#!/usr/bin/env bash
set -Eeuo pipefail

usage() {
  printf '%s\n' \
    '使い方: scripts/test-result.sh write <output> <target> <status> <started> <finished> <log>' >&2
  exit 2
}

write_result() {
  (($# == 6)) || usage

  local output="$1"
  local target="$2"
  local status="$3"
  local started="$4"
  local finished="$5"
  local log="$6"
  local result='成功'

  if [[ "$status" != 0 ]]; then
    result='失敗'
  fi

  mkdir -p "$(dirname "$output")"
  {
    printf '# LWS テスト結果\n\n'
    printf -- '- 実行日: %s\n' "$(date --iso-8601=seconds)"
    printf -- '- 対象: `%s`\n' "$target"
    printf -- '- 結果: **%s**\n' "$result"
    printf -- '- 終了status: `%s`\n' "$status"
    printf -- '- 開始: `%s`\n' "$started"
    printf -- '- 終了: `%s`\n' "$finished"
    printf '\n## 実行ログ\n\n'
    printf '%s\n' '```text'
    sed 's/\r$//' "$log"
    printf '%s\n' '```'
    printf '\n## テスト項目別結果\n\n'
    printf '| ID | 結果 | 内容 |\n'
    printf '|---|---|---|\n'
    if ! grep -q '^TEST_ITEM|' "$log"; then
      printf '| - | 未出力 | 項目別結果がありません |\n'
    else
      while IFS='|' read -r _ id item_status description; do
        printf '| `%s` | %s | %s |\n' "$id" "$item_status" "$description"
      done < <(grep '^TEST_ITEM|' "$log")
    fi
  } >"$output"
}

main() {
  (($# >= 1)) || usage

  case "$1" in
    write)
      shift
      write_result "$@"
      ;;
    *)
      usage
      ;;
  esac
}

main "$@"
