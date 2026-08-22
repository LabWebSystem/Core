#!/usr/bin/env bash
set -Eeuo pipefail

readonly ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
readonly TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

repository="$TMP/valid.git"
git_config="$TMP/gitconfig"
clone="$TMP/clone"
url='https://github.com/test/lws-valid'

"$ROOT/scripts/test-app-fixture.sh" prepare \
  valid \
  "$repository" \
  "$git_config" \
  "$url" \
  >"$TMP/prepare.log"

GIT_CONFIG_GLOBAL="$git_config" \
  git clone --quiet --branch main "$url" "$clone"

test -f "$clone/compose.yaml"
test -f "$clone/lws.manifest.yaml"
test "$(git -C "$clone" rev-parse --abbrev-ref HEAD)" = main
cmp "$ROOT/test/apps/valid/compose.yaml" "$clone/compose.yaml"
cmp "$ROOT/test/apps/valid/lws.manifest.yaml" "$clone/lws.manifest.yaml"
test -d "$repository"

rm -rf "$repository" "$clone" "$git_config"
test ! -e "$repository"
test ! -e "$clone"
test ! -e "$git_config"

printf 'テストfixture基盤: 成功\n'
