#!/usr/bin/env bash
set -Eeuo pipefail

readonly ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
readonly FIXTURE_ROOT="$ROOT/test/apps"

FIXTURE_WORK=""

die() {
  printf '%s\n' "$*" >&2
  exit 1
}

usage() {
  cat >&2 <<'EOF'
使い方:
  scripts/test-app-fixture.sh prepare <fixture> <repository> <git-config> <repository-url>

fixture:
  test/apps配下のfixture名

repository:
  作成する一時bare Git repositoryのパス。既存パスは指定しない

git-config:
  URL書き換えを追加するテスト用Git configのパス

repository-url:
  LWSへ渡すGitHub形式のURL
EOF
  exit 2
}

prepare() {
  (($# == 4)) || usage

  local fixture="$1"
  local repository="$2"
  local git_config="$3"
  local repository_url="$4"
  local source="$FIXTURE_ROOT/$fixture"
  local work=""

  [[ "$fixture" != */* && "$fixture" != .* ]] ||
    die 'fixture名はtest/apps直下の名前を指定してください'
  [[ -d "$source" ]] || die "fixtureが見つかりません: $fixture"
  [[ -f "$source/compose.yaml" ]] || die "compose.yamlがありません: $fixture"
  [[ -f "$source/lws.manifest.yaml" ]] || die "lws.manifest.yamlがありません: $fixture"
  [[ "$repository_url" == https://github.com/* ]] ||
    die 'repository-urlはGitHub HTTPS URLを指定してください'
  [[ ! -e "$repository" ]] || die "repositoryが既に存在します: $repository"

  mkdir -p "$(dirname "$repository")" "$(dirname "$git_config")"
  work="$(mktemp -d)"
  FIXTURE_WORK="$work"

  git init --quiet "$work"
  git -C "$work" checkout --quiet -b main
  git -C "$work" config user.name 'LWSテストfixture'
  git -C "$work" config user.email 'lws-test-fixture@example.invalid'
  cp -a "$source/." "$work/"
  git -C "$work" add -- compose.yaml lws.manifest.yaml
  git -C "$work" commit --quiet -m 'テストアプリfixtureを追加'
  git clone --quiet --bare "$work" "$repository"

  git config --file "$git_config" "url.${repository}.insteadOf" "$repository_url"

  printf '一時Git repositoryを作成しました: %s\n' "$fixture"
  printf 'LWSへ渡すURL: %s\n' "$repository_url"
  printf 'ref: main\n'
  printf 'テスト用Git config: %s\n' "$git_config"
}

cleanup() {
  [[ -z "$FIXTURE_WORK" ]] || rm -rf "$FIXTURE_WORK"
}

trap cleanup EXIT

main() {
  (($# >= 1)) || usage

  case "$1" in
    prepare)
      shift
      prepare "$@"
      ;;
    *)
      usage
      ;;
  esac
}

main "$@"
