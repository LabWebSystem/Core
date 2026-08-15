#!/usr/bin/env bash
set -Eeuo pipefail

readonly ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
readonly LWS_WORKFLOW="release-lws.yml"
readonly SDK_WORKFLOW="release-sdk.yml"

FORCE=false
RELEASE_CORE=false
RELEASE_SDK=false
RELEASE_ALL=false

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
        RELEASE_ALL=true
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

prepare_core_release() {
  local repository="$1"
  local version tag
  version="$("$ROOT/scripts/version.sh" core)"
  tag="lws-v$version"

  delete_existing_core_release "$repository" "$tag"
  printf '%s\n' "$tag"
}

prepare_sdk_release() {
  local version tag
  version="$("$ROOT/scripts/version.sh" sdk)"
  tag="sdk-v$version"

  ensure_sdk_tag_is_new "$tag"
  printf '%s\n' "$tag"
}

release_core() {
  local repository="$1"
  local tag="$2"
  local commit run_id

  push_release_tag "$tag"
  commit="$(git -C "$ROOT" rev-parse "$tag^{commit}")"
  run_id="$(find_workflow_run "$repository" "$LWS_WORKFLOW" "$commit")" || die 'CoreリリースWorkflowを確認できません'
  gh run watch "$run_id" --repo "$repository" --exit-status
}

release_sdk() {
  local repository="$1"
  local tag="$2"
  local commit run_id

  push_release_tag "$tag"
  commit="$(git -C "$ROOT" rev-parse "$tag^{commit}")"
  run_id="$(find_workflow_run "$repository" "$SDK_WORKFLOW" "$commit")" || die 'SDKリリースWorkflowを確認できません'
  gh run watch "$run_id" --repo "$repository" --exit-status
}

release_all() {
  local repository="$1"
  local core_tag sdk_tag core_pid sdk_pid
  local core_status=0 sdk_status=0

  core_tag="$(prepare_core_release "$repository")"
  sdk_tag="$(prepare_sdk_release)"

  (release_core "$repository" "$core_tag") 2>&1 | sed -u 's/^/[LWS] /' &
  core_pid=$!
  (release_sdk "$repository" "$sdk_tag") 2>&1 | sed -u 's/^/[SDK] /' &
  sdk_pid=$!

  wait "$core_pid" || core_status=$?
  wait "$sdk_pid" || sdk_status=$?

  ((core_status == 0 && sdk_status == 0)) || return 1
}

main() {
  parse_args "$@"
  require_command git
  require_command gh
  validate_repository_state

  local repository
  repository="$(resolve_repository)"

  if [[ "$RELEASE_ALL" == true ]]; then
    release_all "$repository"
    return
  fi

  if [[ "$RELEASE_CORE" == true ]]; then
    release_core "$repository" "$(prepare_core_release "$repository")"
  fi

  if [[ "$RELEASE_SDK" == true ]]; then
    release_sdk "$repository" "$(prepare_sdk_release)"
  fi
}

main "$@"
