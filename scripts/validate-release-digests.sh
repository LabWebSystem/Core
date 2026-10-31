#!/usr/bin/env bash
set -Eeuo pipefail

for name in LWS_BACKEND_DIGEST LWS_DASHBOARD_DIGEST LWS_AVAHI_DIGEST; do
  value="${!name:-}"
  if [[ ! "$value" =~ ^sha256:[0-9a-f]{64}$ ]]; then
    printf '%sは有効なsha256 digestではありません\n' "$name" >&2
    exit 1
  fi
  if [[ "$name" == 'LWS_AVAHI_DIGEST' && "$value" == 'sha256:0000000000000000000000000000000000000000000000000000000000000000' ]]; then
    printf 'LWS_AVAHI_DIGESTにplaceholder値は指定できません\n' >&2
    exit 1
  fi
done
