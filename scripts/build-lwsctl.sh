#!/usr/bin/env bash
set -Eeuo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUTPUT="$ROOT/bin/lwsctl"
GOOS_VALUE="${GOOS:-$(go env GOOS)}"
GOARCH_VALUE="${GOARCH:-$(go env GOARCH)}"
while (($#)); do
  case "$1" in
    --output) OUTPUT="${2:-}"; shift 2 ;;
    --goos) GOOS_VALUE="${2:-}"; shift 2 ;;
    --goarch) GOARCH_VALUE="${2:-}"; shift 2 ;;
    *) printf '使い方: scripts/build-lwsctl.sh [--output パス] [--goos OS] [--goarch アーキテクチャ]\n' >&2; exit 2 ;;
  esac
done
[[ -n "$OUTPUT" && -n "$GOOS_VALUE" && -n "$GOARCH_VALUE" ]] || { printf 'ビルド出力先、OS、アーキテクチャを指定してください\n' >&2; exit 2; }
mkdir -p "$(dirname "$OUTPUT")"
CGO_ENABLED=0 GOOS="$GOOS_VALUE" GOARCH="$GOARCH_VALUE" go build -trimpath -ldflags='-s -w' -o "$OUTPUT" ./cmd/lwsctl
