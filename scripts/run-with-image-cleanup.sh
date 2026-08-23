#!/usr/bin/env bash
set -Eeuo pipefail

if [[ "$#" -eq 0 ]]; then
  printf '使い方: scripts/run-with-image-cleanup.sh <command> [args...]\n' >&2
  exit 2
fi

tmp="$(mktemp -d)"

cleanup() {
  rm -rf "$tmp"
}
trap cleanup EXIT

docker image ls --all --quiet --no-trunc | sort -u >"$tmp/images-before"

status=0
if "$@"; then
  :
else
  status=$?
fi

docker image ls --all --quiet --no-trunc | sort -u >"$tmp/images-after"

cleanup_status=0
while IFS= read -r image; do
  [[ -z "$image" ]] && continue
  if docker image rm "$image"; then
    printf 'テストで取得したDocker imageを削除しました: %s\n' "$image"
  else
    printf 'テストで取得したDocker imageを削除できません: %s\n' "$image" >&2
    cleanup_status=1
  fi
done < <(comm -13 "$tmp/images-before" "$tmp/images-after")

if [[ "$status" -ne 0 ]]; then
  exit "$status"
fi
exit "$cleanup_status"
