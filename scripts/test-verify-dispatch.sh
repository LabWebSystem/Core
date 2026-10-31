#!/usr/bin/env bash
set -Eeuo pipefail

readonly ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
readonly TMP="$(mktemp -d)"
readonly RUN_ID='501'
readonly HEAD_SHA='0123456789012345678901234567890123456789'

cleanup() {
  rm -rf "$TMP" "$ROOT/test/result/actions/$RUN_ID"
}
trap cleanup EXIT

mkdir -p "$TMP/bin" "$TMP/state"

cat >"$TMP/bin/git" <<'EOF'
#!/usr/bin/env bash
case " $* " in
  *' status --porcelain --untracked-files=all '*)
    [[ "${LWS_VERIFY_TEST_DIRTY:-0}" != 1 ]] || printf ' M tracked-file\n'
    ;;
  *' branch --show-current '*)
    printf 'feature/verify\n'
    ;;
  *' rev-parse HEAD '*)
    printf '%s\n' "$LWS_VERIFY_TEST_HEAD_SHA"
    ;;
  *' rev-parse --abbrev-ref --symbolic-full-name @{upstream} '*)
    printf 'origin/feature/verify\n'
    ;;
  *' ls-remote --heads origin refs/heads/feature/verify '*)
    printf '%s\trefs/heads/feature/verify\n' "${LWS_VERIFY_TEST_REMOTE_SHA:-$LWS_VERIFY_TEST_HEAD_SHA}"
    ;;
  *)
    printf 'fake git does not handle: %s\n' "$*" >&2
    exit 2
    ;;
esac
EOF

cat >"$TMP/bin/gh" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" >>"$LWS_VERIFY_TEST_LOG"
case " $* " in
  *' auth status '*)
    [[ "${LWS_VERIFY_TEST_AUTH:-1}" == 1 ]]
    ;;
  *' repo view '*)
    printf 'LabWebSystem/Core\n'
    ;;
  *' run list '*)
    if [[ -e "$LWS_VERIFY_TEST_STATE/dispatched" ]]; then
      request_id="$(cat "$LWS_VERIFY_TEST_STATE/request_id")"
      printf '[{"databaseId":%s,"headSha":"%s","createdAt":"2026-10-31T00:00:01Z","url":"https://example.invalid/run/%s","displayTitle":"品質検証 full / feature/verify / %s"}]\n' \
        "$LWS_VERIFY_TEST_RUN_ID" "$LWS_VERIFY_TEST_HEAD_SHA" "$LWS_VERIFY_TEST_RUN_ID" "$request_id"
    else
      printf '[]\n'
    fi
    ;;
  *' workflow run '*)
    for argument in "$@"; do
      if [[ "$argument" == request_id=* ]]; then
        printf '%s\n' "${argument#request_id=}" >"$LWS_VERIFY_TEST_STATE/request_id"
      fi
    done
    touch "$LWS_VERIFY_TEST_STATE/dispatched"
    ;;
  *' run view '*)
    printf 'https://example.invalid/run/%s\n' "$LWS_VERIFY_TEST_RUN_ID"
    ;;
  *' run watch '*)
    exit "${LWS_VERIFY_TEST_WATCH_STATUS:-0}"
    ;;
  *' run download '*)
    destination=''
    while (($#)); do
      if [[ "$1" == --dir ]]; then
        destination="$2"
        shift 2
      else
        shift
      fi
    done
    mkdir -p "$destination/quality-summary"
    printf 'test artifact\n' >"$destination/quality-summary/quality-summary.md"
    ;;
  *)
    printf 'fake gh does not handle: %s\n' "$*" >&2
    exit 2
    ;;
esac
EOF

chmod +x "$TMP/bin/git" "$TMP/bin/gh"

run_verify() {
  local expected_status="$1"
  local output="$2"
  local actual_status=0

  PATH="$TMP/bin:$PATH" \
  LWS_VERIFY_TEST_DIRTY="${LWS_VERIFY_TEST_DIRTY:-0}" \
  LWS_VERIFY_TEST_AUTH="${LWS_VERIFY_TEST_AUTH:-1}" \
  LWS_VERIFY_TEST_HEAD_SHA="$HEAD_SHA" \
  LWS_VERIFY_TEST_REMOTE_SHA="${LWS_VERIFY_TEST_REMOTE_SHA:-$HEAD_SHA}" \
  LWS_VERIFY_TEST_RUN_ID="$RUN_ID" \
  LWS_VERIFY_TEST_WATCH_STATUS="${LWS_VERIFY_TEST_WATCH_STATUS:-0}" \
  LWS_VERIFY_TEST_LOG="$TMP/gh.log" \
  LWS_VERIFY_TEST_STATE="$TMP/state" \
    "$ROOT/scripts/verify.sh" full >"$output" 2>&1 || actual_status=$?

  if [[ "$actual_status" != "$expected_status" ]]; then
    cat "$output" >&2
    printf 'verify dispatcherの終了statusが不正です（期待: %s、実際: %s）\n' \
      "$expected_status" "$actual_status" >&2
    return 1
  fi
}

test_dirty_tree_rejected() {
  rm -f "$TMP/state/dispatched"
  LWS_VERIFY_TEST_DIRTY=1 run_verify 1 "$TMP/dirty.log"
  ! grep -q 'workflow run' "$TMP/gh.log" 2>/dev/null
  grep -q '未コミットの変更' "$TMP/dirty.log" || { cat "$TMP/dirty.log" >&2; return 1; }
}

test_unpushed_commit_rejected() {
  rm -f "$TMP/state/dispatched"
  LWS_VERIFY_TEST_REMOTE_SHA='ffffffffffffffffffffffffffffffffffffffff' run_verify 1 "$TMP/unpushed.log"
  ! grep -q 'workflow run' "$TMP/gh.log" 2>/dev/null
  grep -q '追跡先と一致しません' "$TMP/unpushed.log"
}

test_authentication_required() {
  rm -f "$TMP/state/dispatched"
  LWS_VERIFY_TEST_AUTH=0 run_verify 1 "$TMP/auth.log"
  ! grep -q 'workflow run' "$TMP/gh.log" 2>/dev/null
  grep -q 'gh auth login' "$TMP/auth.log"
}

test_dispatch_wait_and_artifacts() {
  rm -f "$TMP/state/dispatched" "$TMP/gh.log"
  LWS_VERIFY_TEST_WATCH_STATUS=0 run_verify 0 "$TMP/success.log"
  grep -q 'quality.yml.*--ref feature/verify.*--field profile=full' "$TMP/gh.log"
  grep -q -- '--field request_id=verify-' "$TMP/gh.log"
  grep -q "run watch $RUN_ID" "$TMP/gh.log"
  grep -q "run download $RUN_ID" "$TMP/gh.log"
  grep -q '実行URL: https://example.invalid/run/501' "$TMP/success.log"
  test -f "$ROOT/test/result/actions/$RUN_ID/quality-summary/quality-summary.md"
}

test_workflow_failure_propagated() {
  rm -f "$TMP/gh.log" "$TMP/state/dispatched"
  LWS_VERIFY_TEST_WATCH_STATUS=1 run_verify 1 "$TMP/failure.log"
  grep -q "run download $RUN_ID" "$TMP/gh.log"
  grep -q '実行URL:' "$TMP/failure.log"
}

test_dirty_tree_rejected
test_unpushed_commit_rejected
test_authentication_required
test_dispatch_wait_and_artifacts
test_workflow_failure_propagated
printf 'GitHub Actions検証dispatcherテスト: 成功\n'
