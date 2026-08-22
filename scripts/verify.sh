#!/usr/bin/env bash
set -Euo pipefail

readonly ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
readonly PROFILE="${1:-fast}"
readonly RESULT_DIR="$ROOT/test/result"
readonly RESULT_FILE="$RESULT_DIR/$(date +%F)-verify-result.md"

TMP=""
STARTED=""
OVERALL_STATUS=0
declare -a RECORDS=()
declare -a LOGS=()

usage() {
  printf '使い方: scripts/verify.sh [fast|qa|release]\n' >&2
  exit 2
}

append_record() {
  local id="$1"
  local status="$2"
  local description="$3"
  local elapsed="$4"

  RECORDS+=("$id|$status|$description|$elapsed")
}

run_step() {
  local id="$1"
  local description="$2"
  shift 2
  local log="$TMP/$id.log"
  local started finished elapsed

  started="$(date +%s)"
  if "$@" >"$log" 2>&1; then
    finished="$(date +%s)"
    elapsed="$((finished - started))秒"
    append_record "$id" '成功' "$description" "$elapsed"
    LOGS+=("$id|$description|$log")
    printf 'VERIFY_ITEM|%s|成功|%s|%s\n' "$id" "$description" "$elapsed"
    return 0
  fi

  finished="$(date +%s)"
  elapsed="$((finished - started))秒"
  append_record "$id" '失敗' "$description" "$elapsed"
  LOGS+=("$id|$description|$log")
  printf 'VERIFY_ITEM|%s|失敗|%s|%s\n' "$id" "$description" "$elapsed" >&2
  return 1
}

skip_step() {
  local id="$1"
  local description="$2"

  append_record "$id" '未実行' "$description" '-'
  printf 'VERIFY_ITEM|%s|未実行|%s|-\n' "$id" "$description"
}

run_fast() {
  local status=0

  run_step 'VFY-001' '静的検査' mise run lint || status=1
  run_step 'VFY-002' 'Core高速テスト' mise run test fast || status=1
  run_step 'VFY-003' 'SDKテストとbuild' mise run test-sdk || status=1
  run_step 'VFY-004' 'Dashboardイメージbuild' mise run test-dashboard || status=1
  return "$status"
}

run_qa() {
  local status=0

  run_step 'VFY-005' 'Core統合テスト' mise run test || status=1
  run_step 'VFY-006' '自己完結Robot QA' mise run qa-current || status=1
  run_step 'VFY-007' '隔離ライフサイクルRobot QA' mise run qa-lifecycle || status=1
  run_step 'VFY-008' 'QAテスト項目ドキュメント生成' mise run qa-docs || status=1
  return "$status"
}

append_robot_artifact() {
  local directory="$1"
  local label="$2"

  if [[ -f "$RESULT_DIR/$directory/report.html" ]]; then
    printf -- '- [%s report](%s/report.html) / [%s log](%s/log.html)\n' "$label" "$directory" "$label" "$directory"
  fi
}

has_step() {
  local expected="$1"
  local record

  for record in "${RECORDS[@]}"; do
    [[ "$record" == "$expected"'|'* ]] && return 0
  done
  return 1
}

write_result() {
  local status="$1"
  local result='成功'
  local finished
  local record id item_status description elapsed log_id log_description log_path

  if [[ "$status" != 0 ]]; then
    result='失敗'
  fi
  finished="$(date --iso-8601=seconds)"
  mkdir -p "$RESULT_DIR"

  {
    printf '# LWS 品質ゲート結果\n\n'
    printf -- '- プロファイル: `%s`\n' "$PROFILE"
    printf -- '- 結果: **%s**\n' "$result"
    printf -- '- 終了status: `%s`\n' "$status"
    printf -- '- 開始: `%s`\n' "$STARTED"
    printf -- '- 終了: `%s`\n' "$finished"
    printf '\n## 検証項目\n\n'
    printf '| ID | 結果 | 内容 | 所要時間 |\n'
    printf '|---|---|---|---|\n'
    for record in "${RECORDS[@]}"; do
      IFS='|' read -r id item_status description elapsed <<<"$record"
      printf '| `%s` | %s | %s | %s |\n' "$id" "$item_status" "$description" "$elapsed"
    done
    printf '\n## Robot Frameworkの詳細結果\n\n'
    if has_step 'VFY-006'; then
      append_robot_artifact 'robot-current' '自己完結QA'
    fi
    if has_step 'VFY-007'; then
      append_robot_artifact 'robot-lifecycle' '隔離ライフサイクルQA'
    fi
    if ! has_step 'VFY-006' && ! has_step 'VFY-007'; then
      printf -- '- このプロファイルではRobot Frameworkを実行していません。\n'
    fi
    printf '\n## 実行ログ\n'
    for record in "${LOGS[@]}"; do
      IFS='|' read -r log_id log_description log_path <<<"$record"
      printf '\n### %s: %s\n\n' "$log_id" "$log_description"
      printf '```text\n'
      sed 's/\r$//' "$log_path"
      printf '```\n'
    done
  } >"$RESULT_FILE"
}

cleanup() {
  local status=$?

  if [[ -n "$TMP" ]]; then
    write_result "$status"
    rm -rf "$TMP"
  fi
}

main() {
  case "$PROFILE" in
    fast|qa|release) ;;
    *) usage ;;
  esac

  TMP="$(mktemp -d)"
  STARTED="$(date --iso-8601=seconds)"
  trap cleanup EXIT

  if ! run_fast; then
    OVERALL_STATUS=1
  fi

  if [[ "$PROFILE" == qa || "$PROFILE" == release ]]; then
    if [[ "$OVERALL_STATUS" == 0 ]]; then
      run_qa || OVERALL_STATUS=1
    else
      skip_step 'VFY-005' 'Core統合テスト（fast失敗のため）'
      skip_step 'VFY-006' '自己完結Robot QA（fast失敗のため）'
      skip_step 'VFY-007' '隔離ライフサイクルRobot QA（fast失敗のため）'
      skip_step 'VFY-008' 'QAテスト項目ドキュメント生成（fast失敗のため）'
    fi
  fi

  if [[ "$PROFILE" == release ]]; then
    if [[ "$OVERALL_STATUS" == 0 ]]; then
      run_step 'VFY-009' 'リリースパッケージ生成' mise run package || OVERALL_STATUS=1
    else
      skip_step 'VFY-009' 'リリースパッケージ生成（前提検証失敗のため）'
    fi
  fi

  printf 'VERIFY_RESULT|%s|%s\n' "$PROFILE" "$([[ "$OVERALL_STATUS" == 0 ]] && printf '成功' || printf '失敗')"
  printf 'VERIFY_REPORT|%s\n' "$RESULT_FILE"
  return "$OVERALL_STATUS"
}

main "$@"
