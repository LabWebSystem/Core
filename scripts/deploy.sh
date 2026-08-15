#!/usr/bin/env bash
set -Eeuo pipefail

readonly ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
readonly WORKFLOW="release-lws.yml"

VERSION=""
FORCE=false

usage() {
  printf '使い方: mise run deploy --version x.y.z [--force]\n'
}

die() {
  printf '%s\n' "$*" >&2
  exit 1
}

require_command() {
  command -v "$1" >/dev/null 2>&1 || die "$1 CLIが必要です"
}

parse_args() {
  while (($#)); do
    case "$1" in
      --version)
        (($# >= 2)) || die '--versionには値が必要です'
        VERSION="$2"
        shift 2
        ;;
      --force)
        FORCE=true
        shift
        ;;
      -h | --help)
        usage
        exit 0
        ;;
      *)
        usage >&2
        exit 2
        ;;
    esac
  done
}

validate_version() {
  [[ "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] ||
    die 'バージョンはx.y.z形式で指定してください'
}

validate_repository_state() {
  local branch
  branch="$(git -C "$ROOT" branch --show-current)"

  [[ "$branch" == "main" ]] ||
    die 'リリースはmainブランチからのみ実行できます'
}

resolve_repository() {
  if [[ -n "${LWS_REPOSITORY:-}" ]]; then
    printf '%s\n' "$LWS_REPOSITORY"
    return
  fi

  gh repo view \
    --json nameWithOwner \
    --jq '.nameWithOwner'
}

delete_existing_release() {
  local repository="$1"
  local tag="$2"

  gh release view "$tag" --repo "$repository" >/dev/null 2>&1 ||
    return 0

  if [[ "$FORCE" != true ]]; then
    local answer
    read -r -p "リリース $tag は存在します。置き換えますか? [y/N]: " answer

    [[ "$answer" =~ ^([yY]|yes|YES)$ ]] ||
      die 'デプロイをキャンセルしました'
  fi

  gh release delete "$tag" \
    --repo "$repository" \
    --yes
}

push_release_tag() {
  local tag="$1"

  git -C "$ROOT" tag -f "$tag"
  git -C "$ROOT" push origin "+$tag"
}

find_workflow_run() {
  local repository="$1"
  local commit="$2"
  local run_id=""

  for _ in {1..15}; do
    run_id="$(
      gh run list \
        --repo "$repository" \
        --workflow "$WORKFLOW" \
        --event push \
        --commit "$commit" \
        --limit 1 \
        --json databaseId \
        --jq '.[0].databaseId'
    )"

    if [[ -n "$run_id" ]]; then
      printf '%s\n' "$run_id"
      return 0
    fi

    sleep 2
  done

  return 1
}

main() {
  parse_args "$@"

  require_command git
  require_command gh

  validate_version
  validate_repository_state

  local repository
  local tag
  local tag_commit
  local run_id

  repository="$(resolve_repository)"
  tag="lws-v$VERSION"

  delete_existing_release "$repository" "$tag"
  push_release_tag "$tag"

  tag_commit="$(git -C "$ROOT" rev-parse "$tag^{commit}")"

  run_id="$(find_workflow_run "$repository" "$tag_commit")" ||
    die 'リリースWorkflowを確認できません'

  gh run watch "$run_id" \
    --repo "$repository" \
    --exit-status
}

main "$@"