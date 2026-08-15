#!/usr/bin/env bash
set -Eeuo pipefail

readonly ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
readonly LWS_WORKFLOW="release-lws.yml"
readonly SDK_WORKFLOW="release-sdk.yml"

FORCE=false
RELEASE_CORE=false
RELEASE_SDK=false

usage() {
  printf '使い方: mise run release <core|sdk|all>... [--force]\n'
}

die() {
  printf '%s\n' "$*" >&2
  exit 1
}

require_command() {
  command -v "$1" >/dev/null 2>&1 || die "$1 CLIが必要です"
}

parse_args() {
  (($# > 0)) || {
    usage >&2
    exit 2
  }

  while (($#)); do
    case "$1" in
      core)
        RELEASE_CORE=true
        ;;
      sdk)
        RELEASE_SDK=true
        ;;
      all)
        RELEASE_CORE=true
        RELEASE_SDK=true
        ;;
      --force)
        FORCE=true
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
    shift
  done
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

  gh repo view --json nameWithOwner --jq '.nameWithOwner'
}

delete_existing_core_release() {
  local repository="$1"
  local tag="$2"

  gh release view "$tag" --repo "$repository" >/dev/null 2>&1 || return 0

  if [[ "$FORCE" != true ]]; then
    local answer
    read -r -p "リリース $tag は存在します。置き換えますか? [y/N]: " answer
    [[ "$answer" =~ ^([yY]|yes|YES)$ ]] || die 'リリースをキャンセルしました'
  fi

  gh release delete "$tag" --repo "$repository" --yes
}

ensure_sdk_tag_is_new() {
  local tag="$1"

  if git -C "$ROOT" ls-remote --exit-code --tags origin "refs/tags/$tag" \
    >/dev/null 2>&1; then
    die "SDKタグ $tag は既に存在します。GitHub Packagesの公開済みバージョンは再公開できません"
  fi
}

push_release_tag() {
  local tag="$1"
  git -C "$ROOT" tag -f "$tag"
  git -C "$ROOT" push origin "+$tag"
}

find_workflow_run() {
  local repository="$1"
  local workflow="$2"
  local commit="$3"
  local run_id=""

  for _ in {1..15}; do
    run_id="$(gh run list --repo "$repository" --workflow "$workflow" --event push --commit "$commit" --limit 1 --json databaseId --jq '.[0].databaseId')"
    if [[ -n "$run_id" ]]; then
      printf '%s\n' "$run_id"
      return 0
    fi
    sleep 2
  done

  return 1
}

release_core() {
  local repository="$1"
  local version tag commit run_id
  version="$("$ROOT/scripts/version.sh" core)"
  tag="lws-v$version"

  delete_existing_core_release "$repository" "$tag"
  push_release_tag "$tag"
  commit="$(git -C "$ROOT" rev-parse "$tag^{commit}")"
  run_id="$(find_workflow_run "$repository" "$LWS_WORKFLOW" "$commit")" || die 'CoreリリースWorkflowを確認できません'
  gh run watch "$run_id" --repo "$repository" --exit-status
}

release_sdk() {
  local repository="$1"
  local version tag commit run_id
  version="$("$ROOT/scripts/version.sh" sdk)"
  tag="sdk-v$version"

  ensure_sdk_tag_is_new "$tag"
  push_release_tag "$tag"
  commit="$(git -C "$ROOT" rev-parse "$tag^{commit}")"
  run_id="$(find_workflow_run "$repository" "$SDK_WORKFLOW" "$commit")" || die 'SDKリリースWorkflowを確認できません'
  gh run watch "$run_id" --repo "$repository" --exit-status
}

main() {
  parse_args "$@"
  require_command git
  require_command gh
  validate_repository_state

  local repository
  repository="$(resolve_repository)"

  [[ "$RELEASE_CORE" != true ]] || release_core "$repository"
  [[ "$RELEASE_SDK" != true ]] || release_sdk "$repository"
}

main "$@"
