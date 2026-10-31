#!/usr/bin/env bash

timing_begin() {
  LWS_TIMING_LABEL="$1"
  LWS_TIMING_START="$SECONDS"
}

timing_end() {
  local status="${1:-0}"
  local elapsed=$((SECONDS - LWS_TIMING_START))
  printf '[timing] %s: %ss (exit %s)\n' "$LWS_TIMING_LABEL" "$elapsed" "$status"
  LWS_TIMING_LABEL=""
  LWS_TIMING_START=""
}

timing_report_on_exit() {
  local status=$?
  if [[ -n "${LWS_TIMING_LABEL:-}" ]]; then
    timing_end "$status"
  fi
  return 0
}
