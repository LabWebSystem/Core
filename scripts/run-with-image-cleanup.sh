#!/usr/bin/env bash
set -Eeuo pipefail

if [[ "$#" -eq 0 ]]; then
  printf '使い方: scripts/run-with-image-cleanup.sh <command> [args...]\n' >&2
  exit 2
fi

tmp="$(mktemp -d)"
run_id="${tmp##*.}"

cleanup() {
  rm -rf "$tmp"
}
trap cleanup EXIT

export LWS_TEST_RUN_ID="$run_id"

status=0
if "$@"; then
  :
else
  status=$?
fi

cleanup_status=0
if ! docker image ls --all --quiet --no-trunc --filter "label=com.labwebsystem.test-run=$run_id" >"$tmp/images"; then
  printf 'このテストが作成したDocker imageを確認できませんでした\n' >&2
  cleanup_status=1
fi
while IFS= read -r image; do
  [[ -z "$image" ]] && continue
  if docker image rm "$image"; then
    printf 'このテストが作成したDocker imageを削除しました: %s\n' "$image"
  else
    printf 'このテストが作成したDocker imageを削除できません: %s\n' "$image" >&2
    cleanup_status=1
  fi
done <"$tmp/images"

if [[ "$status" -ne 0 ]]; then
  exit "$status"
fi
exit "$cleanup_status"
