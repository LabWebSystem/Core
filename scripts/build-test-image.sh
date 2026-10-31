#!/usr/bin/env bash
set -Eeuo pipefail

if [[ "$#" -ne 4 ]]; then
  printf '使い方: scripts/build-test-image.sh <Dockerfile> <image> <cache-scope> <context>\n' >&2
  exit 2
fi

readonly DOCKERFILE="$1"
readonly IMAGE="$2"
readonly CACHE_SCOPE="$3"
readonly CONTEXT="$4"

if [[ -n "${ACTIONS_RUNTIME_TOKEN:-}" && ( -n "${ACTIONS_CACHE_URL:-}" || -n "${ACTIONS_RESULTS_URL:-}" ) ]]; then
  args=(buildx build --load --allow network.host --network host --file "$DOCKERFILE" --tag "$IMAGE")
  args+=(--cache-from "type=gha,scope=$CACHE_SCOPE")
  args+=(--cache-to "type=gha,scope=$CACHE_SCOPE,mode=max,ignore-error=true")
  docker "${args[@]}" "$CONTEXT"
else
  docker build --network host --file "$DOCKERFILE" --tag "$IMAGE" "$CONTEXT"
fi
