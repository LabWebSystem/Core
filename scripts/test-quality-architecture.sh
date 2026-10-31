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
  local mise_lib="$ROOT/scripts/mise.lib.toml"

  require_match 'architecture' "$ROOT/scripts/test.sh"
  require_match 'all\|fast\|fixtures' "$ROOT/scripts/test.sh"
  require_match '^\[task_config\]$' "$mise_file"
  require_match '^includes = \["scripts/mise[.]lib[.]toml"\]$' "$mise_file"
  require_match '^\[tasks\.verify\]$' "$mise_file"
  require_match '^\[tasks\.dev\]$' "$mise_file"
  require_match '^\[tasks\.build\]$' "$mise_file"
  require_match '^\[tasks\.package\]$' "$mise_file"
  require_match '^\[tasks\.version\]$' "$mise_file"
  require_match '^\[tasks\.release\]$' "$mise_file"
  require_match 'scripts/verify\.sh \$\{usage_profile:-fast\}' "$mise_file"
  require_match 'arg "\[profile\]" help="検証範囲（fast、full。qaとreleaseはfullの互換指定）"' "$mise_file"
  require_match 'gh workflow run "\$WORKFLOW"' "$ROOT/scripts/verify.sh"
  require_match 'request_id=\$request_id' "$ROOT/scripts/verify.sh"
  require_match 'gh run watch "\$run_id"' "$ROOT/scripts/verify.sh"
  require_match 'gh run download "\$run_id"' "$ROOT/scripts/verify.sh"
  require_match 'git -C .*status --porcelain --untracked-files=all' "$ROOT/scripts/verify.sh"
  require_match '追跡先と一致しません' "$ROOT/scripts/verify.sh"
  require_match 'test-verify-dispatch[.]sh' "$ROOT/scripts/test.sh"
  require_match '^\[lint\]$' "$mise_lib"
  require_match '^\[test\]$' "$mise_lib"
  require_match '^\[test-sdk\]$' "$mise_lib"
  require_match '^\[test-dashboard\]$' "$mise_lib"
  require_match 'run-with-image-cleanup[.]sh docker build' "$mise_lib"
  require_match '^\[qa\]$' "$mise_lib"
  require_match '^\[qa-current\]$' "$mise_lib"
  require_match '^\[qa-live\]$' "$mise_lib"
  require_match '^\[qa-lifecycle\]$' "$mise_lib"
  require_match 'run-with-image-cleanup[.]sh uv run --project qa robot' "$mise_lib"
  require_match 'docker image ls --all --quiet --no-trunc' "$ROOT/scripts/run-with-image-cleanup.sh"
  require_match 'comm -13' "$ROOT/scripts/run-with-image-cleanup.sh"
  require_match 'run-with-image-cleanup[.]sh.*test-infrastructure[.]sh' "$ROOT/scripts/test.sh"
  require_match '^\[qa-docs\]$' "$mise_lib"
  require_match '^\[generate-backend\]$' "$mise_lib"
  require_match '^\[check-openapi\]$' "$mise_lib"
  require_match '^depends = \["dashboard-install"\]$' "$mise_lib"
  reject_match '^\[tasks\.(lint|test|test-sdk|test-dashboard|qa|qa-current|qa-live|qa-lifecycle|qa-docs|generate-backend|check-openapi)\]$' "$mise_file"
}

check_result_boundary() {
  reject_match 'test-result[.]sh|^TEST_(LOG|RESULT|TARGET|STARTED)=' "$ROOT/scripts/test.sh"
}

check_workflows() {
  local ci="$ROOT/.github/workflows/ci.yml"
  local quality="$ROOT/.github/workflows/quality.yml"
  local lws_release="$ROOT/.github/workflows/release-lws.yml"
  local sdk_release="$ROOT/.github/workflows/release-sdk.yml"
  local direct_runner='^[[:space:]]*-[[:space:]]*run:.*(go test|npm( run)? (test|build)|robot|scripts/test\.sh)'

  require_match "branches: \['\*\*'\]" "$ci"
  require_match 'uses: [.]/.github/workflows/quality[.]yml' "$ci"
  require_match 'profile: fast' "$ci"
  require_match 'workflow_call:' "$quality"
  require_match 'workflow_dispatch:' "$quality"
  require_match 'request_id:' "$quality"
  require_match 'default: full' "$quality"
  require_match 'run-name: .*inputs[.]request_id' "$quality"
  require_match_count '^[[:space:]]+- name: ' 11 "$quality"
  require_match_count 'uses: actions/upload-artifact@v7' 10 "$quality"
  require_match 'run: mise run lint' "$quality"
  require_match 'run: mise run test fast' "$quality"
  require_match 'run: mise run test-sdk' "$quality"
  require_match 'run: mise run test-dashboard' "$quality"
  require_match 'run: mise run test$' "$quality"
  require_match 'run: mise run qa-current' "$quality"
  require_match 'run: mise run qa-lifecycle' "$quality"
  require_match 'run: mise run qa-docs' "$quality"
  require_match 'run: mise run test-import-e2e' "$quality"
  require_match 'needs: \[static, core-fast, sdk, dashboard\]' "$quality"
  require_match 'if: always\(\)' "$quality"
  reject_match 'mise run package|VFY-009' "$quality"
  require_match 'uses: [.]/.github/workflows/quality[.]yml' "$lws_release"
  require_match 'profile: full' "$lws_release"
  require_match '^  workflow_dispatch:$' "$lws_release"
  require_match_count 'needs: \[quality\]' 3 "$lws_release"
  require_match_count '^        id: dry_run$' 3 "$lws_release"
  require_match_count '^        if: github[.]event_name == .workflow_dispatch.$' 3 "$lws_release"
  require_match_count 'outputs: type=image,name=' 3 "$lws_release"
  require_match "if: github[.]event_name == 'push'" "$lws_release"
  require_match 'package:[[:space:]]*$' "$lws_release"
  require_match 'needs: \[quality, backend-image, dashboard-image, avahi-image\]' "$lws_release"
  require_match 'release:[[:space:]]*$' "$lws_release"
  require_match "^    if: github[.]event_name == 'push'$" "$lws_release"
  require_match 'needs: \[quality, package, backend-image, dashboard-image, avahi-image\]' "$lws_release"
  require_match 'LWS_RELEASE_MODE: '\''1'\''' "$lws_release"
  require_match 'validate-release-digests[.]sh' "$ROOT/scripts/package.sh"
  require_match 'LWS_AVAHI_DIGEST' "$ROOT/scripts/validate-release-digests.sh"
  require_match '"config", "--format", "json"' "$ROOT/scripts/prepare-lifecycle-compose.py"
  require_match 'context != ROOT' "$ROOT/scripts/prepare-lifecycle-compose.py"
  require_match 'backend\["image"\] = args[.]backend_image' "$ROOT/scripts/prepare-lifecycle-compose.py"
  require_match 'backend\["pull_policy"\] = "build"' "$ROOT/scripts/prepare-lifecycle-compose.py"
  require_match 'avahi\["image"\] = args[.]avahi_image' "$ROOT/scripts/prepare-lifecycle-compose.py"
  require_match 'internal\["name"\] = f"\{args[.]project\}-internal"' "$ROOT/scripts/prepare-lifecycle-compose.py"
  require_match 'docker ps --all --quiet --filter "ancestor=\$image"' "$ROOT/scripts/run-with-image-cleanup.sh"
  require_match 'mise run test-sdk$' "$sdk_release"
  reject_match "$direct_runner" "$ci" "$quality" "$lws_release" "$sdk_release"
}

check_qa_boundary() {
  local direct_runner='go test|npm( run)? test|scripts/test\.sh'

  reject_match "$direct_runner" "$ROOT/qa"
}

main() {
  check_mise_tasks
  check_result_boundary
  check_workflows
  check_qa_boundary
  printf '品質ゲート構造検査: 成功\n'
}

main "$@"
