#!/usr/bin/env bash
set -Eeuo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP="$(mktemp -d)"; trap 'rm -rf "$TMP"' EXIT
mkdir -p "$TMP/bin"
cat >"$TMP/bin/docker" <<'EOF'
#!/bin/sh
printf '%s\n' "$*" >>"${LWS_TEST_LOG:?}"
EOF
chmod +x "$TMP/bin/docker"
export PATH="$TMP/bin:$PATH" LWS_TEST_LOG="$TMP/docker.log" LWS_CONFIG_DIR="$TMP/etc" LWS_STATE_DIR="$TMP/state" LWS_COMPOSE_FILE="$ROOT/infrastructure/compose.yaml" LWS_SKIP_PACKAGE_REMOVE=1
"$ROOT/scripts/lwsctl" start --domain example.internal
grep -q 'compose --project-name lws' "$TMP/docker.log"
grep -q 'LWS_BASE_DOMAIN=example.internal' "$TMP/etc/config.env"
! "$ROOT/scripts/lwsctl" start --domain bad_domain >/dev/null 2>&1
"$ROOT/scripts/lwsctl" stop >/dev/null
printf 'テスト: 成功\n'
