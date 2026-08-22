#!/usr/bin/env bash
set -Eeuo pipefail

readonly ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
readonly FIXTURE_SCRIPT="$ROOT/scripts/test-app-fixture.sh"
readonly TEST_ROOT="$(mktemp -d)"
trap 'rm -rf "$TEST_ROOT"' EXIT

run_prepare() {
  local fixture="$1"
  local repository="$2"
  local git_config="$3"
  local repository_url="$4"

  "$FIXTURE_SCRIPT" prepare \
    "$fixture" \
    "$repository" \
    "$git_config" \
    "$repository_url" \
    >"$TEST_ROOT/prepare.log" \
    2>&1
}

test_tf_v1_001() {
  local root="$1"
  local repository="$root/valid.git"
  local git_config="$root/gitconfig"

  run_prepare valid "$repository" "$git_config" https://github.com/test/lws-valid
  test -d "$repository"
  git --git-dir "$repository" show-ref --verify --quiet refs/heads/main
}

test_tf_v1_002() {
  local root="$1"
  local repository="$root/valid.git"
  local git_config="$root/gitconfig"
  local clone="$root/clone"
  local url=https://github.com/test/lws-valid

  run_prepare valid "$repository" "$git_config" "$url"
  GIT_CONFIG_GLOBAL="$git_config" git clone --quiet --branch main "$url" "$clone"
  test -f "$clone/compose.yaml"
  test -f "$clone/lws.manifest.yaml"
}

test_tf_v1_003() {
  local root="$1"
  local repository="$root/valid.git"
  local git_config="$root/gitconfig"
  local clone="$root/clone"
  local url=https://github.com/test/lws-valid

  run_prepare valid "$repository" "$git_config" "$url"
  GIT_CONFIG_GLOBAL="$git_config" git clone --quiet --branch main "$url" "$clone"
  test "$(git -C "$clone" rev-parse --abbrev-ref HEAD)" = main
  cmp "$ROOT/test/apps/valid/compose.yaml" "$clone/compose.yaml"
  cmp "$ROOT/test/apps/valid/lws.manifest.yaml" "$clone/lws.manifest.yaml"
}

test_tf_v1_004() {
  local root="$1"
  local repository="$root/valid.git"
  local git_config="$root/gitconfig"
  local clone="$root/clone"
  local url=https://github.com/test/lws-valid

  run_prepare valid "$repository" "$git_config" "$url"
  GIT_CONFIG_GLOBAL="$git_config" git clone --quiet --branch main "$url" "$clone"
  test -d "$repository"
  test -d "$clone"
  test -f "$git_config"

  rm -rf "$repository" "$clone" "$git_config"
  test ! -e "$repository"
  test ! -e "$clone"
  test ! -e "$git_config"
}

test_tf_v1_005() {
  local root="$1"
  local repository="$root/invalid.git"
  local git_config="$root/gitconfig"
  local existing="$root/existing.git"

  if run_prepare missing "$repository" "$git_config" https://github.com/test/missing; then
    return 1
  fi
  test ! -e "$repository"

  if run_prepare valid "$repository" "$git_config" https://gitlab.com/test/valid; then
    return 1
  fi
  test ! -e "$repository"

  mkdir -p "$existing"
  printf '既存resource\n' >"$existing/marker"
  if run_prepare valid "$existing" "$git_config" https://github.com/test/existing; then
    return 1
  fi
  test -f "$existing/marker"
}

run_item() {
  local id="$1"
  local description="$2"
  shift 2
  local item_root="$TEST_ROOT/$id"
  mkdir -p "$item_root"

  if "$@" "$item_root"; then
    printf 'TEST_ITEM|%s|成功|%s\n' "$id" "$description"
    return 0
  fi

  printf 'TEST_ITEM|%s|失敗|%s\n' "$id" "$description"
  return 1
}

main() {
  local failed=0

  run_item TF-V1-001 '一時bare Git repositoryを作成する' test_tf_v1_001 || failed=1
  run_item TF-V1-002 'GitHub形式URLでfixtureをcloneする' test_tf_v1_002 || failed=1
  run_item TF-V1-003 'clone先のrefとfixture内容を確認する' test_tf_v1_003 || failed=1
  run_item TF-V1-004 '一時Git repository、clone先、Git configを削除する' test_tf_v1_004 || failed=1
  run_item TF-V1-005 '不正fixture・URL・既存出力先を拒否する' test_tf_v1_005 || failed=1

  if ((failed != 0)); then
    return 1
  fi
  printf 'テストfixture基盤: 成功\n'
}

main "$@"
