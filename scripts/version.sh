#!/usr/bin/env bash
set -Eeuo pipefail

readonly ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
readonly CORE_VERSION_FILE="$ROOT/version/core"

usage() {
  printf '使い方: mise run version [core|sdk] [x.y.z]\n'
}

die() {
  printf '%s\n' "$*" >&2
  exit 1
}

validate_version() {
  [[ "$1" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] ||
    die 'バージョンはx.y.z形式で指定してください'
}

read_core_version() {
  [[ -r "$CORE_VERSION_FILE" ]] ||
    die "Coreバージョンファイルを読み取れません: $CORE_VERSION_FILE"

  local version
  version="$(tr -d '[:space:]' <"$CORE_VERSION_FILE")"
  validate_version "$version"
  printf '%s\n' "$version"
}

read_sdk_version() {
  command -v node >/dev/null 2>&1 || die 'SDKバージョンの操作にはnodeが必要です'

  local version
  version="$(node -p "require('$ROOT/sdk/package.json').version")"
  validate_version "$version"
  printf '%s\n' "$version"
}

write_core_version() {
  local version="$1"
  mkdir -p "$(dirname "$CORE_VERSION_FILE")"
  printf '%s\n' "$version" >"$CORE_VERSION_FILE"
}

write_sdk_version() {
  local version="$1"
  (
    cd "$ROOT/sdk"
    npm version "$version" --no-git-tag-version >/dev/null
  )
}

main() {
  (($# <= 2)) || {
    usage >&2
    exit 2
  }

  local component="${1:-}"
  local version="${2:-}"

  if [[ -z "$component" ]]; then
    [[ -z "$version" ]] || {
      usage >&2
      exit 2
    }

    printf 'core %s\n' "$(read_core_version)"
    printf 'sdk %s\n' "$(read_sdk_version)"
    return
  fi

  if [[ -n "$version" ]]; then
    validate_version "$version"
  fi

  case "$component" in
    core)
      if [[ -n "$version" ]]; then
        write_core_version "$version"
      else
        read_core_version
      fi
      ;;
    sdk)
      if [[ -n "$version" ]]; then
        write_sdk_version "$version"
      else
        read_sdk_version
      fi
      ;;
    *)
      usage >&2
      exit 2
      ;;
  esac
}

main "$@"
