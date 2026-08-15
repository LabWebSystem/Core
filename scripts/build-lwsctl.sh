#!/usr/bin/env bash
set -Eeuo pipefail

readonly ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

OUTPUT="$ROOT/bin/lwsctl"
GOOS_VALUE="${GOOS:-$(go env GOOS)}"
GOARCH_VALUE="${GOARCH:-$(go env GOARCH)}"

usage() {
  printf '使い方: scripts/build-lwsctl.sh [--output パス] [--goos OS] [--goarch アーキテクチャ]\n'
}

die() {
  printf '%s\n' "$*" >&2
  exit 2
}

require_value() {
  (($# >= 2)) && [[ -n "$2" ]] || die "$1には値が必要です"
}

while (($#)); do
  case "$1" in
    --output)
      require_value "$@"
      OUTPUT="$2"
      shift 2
      ;;
    --goos)
      require_value "$@"
      GOOS_VALUE="$2"
      shift 2
      ;;
    --goarch)
      require_value "$@"
      GOARCH_VALUE="$2"
      shift 2
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

mkdir -p "$(dirname "$OUTPUT")"

CGO_ENABLED=0 \
GOOS="$GOOS_VALUE" \
GOARCH="$GOARCH_VALUE" \
  go build \
    -trimpath \
    -ldflags='-s -w' \
    -o "$OUTPUT" \
    ./cmd/lwsctl