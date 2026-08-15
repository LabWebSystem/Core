#!/usr/bin/env bash
set -Eeuo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP="$(mktemp -d)"; trap 'rm -rf "$TMP"' EXIT
mkdir -p "$TMP/bin"
cat >"$TMP/bin/docker" <<'EOF'
#!/bin/sh
printf '%s\n' "$*" >>"${LWS_TEST_LOG:?}"
printf 'LWS_BASE_DOMAIN=%s\n' "${LWS_BASE_DOMAIN:-}" >>"${LWS_TEST_LOG:?}"
EOF
chmod +x "$TMP/bin/docker"
export PATH="$TMP/bin:$PATH" LWS_TEST_LOG="$TMP/docker.log" LWS_CONFIG_DIR="$TMP/etc" LWS_STATE_DIR="$TMP/state" LWS_COMPOSE_FILE="$ROOT/infrastructure/compose.yaml" LWS_SKIP_PACKAGE_REMOVE=1
"$ROOT/scripts/lwsctl" start --domain example.internal
grep -q 'compose --project-name lws' "$TMP/docker.log"
grep -q 'LWS_BASE_DOMAIN=example.internal' "$TMP/etc/config.env"
grep -q 'LWS_BASE_DOMAIN=example.internal' "$TMP/docker.log"
! "$ROOT/scripts/lwsctl" start --domain bad_domain >/dev/null 2>&1
"$ROOT/scripts/lwsctl" stop >/dev/null

INSTALLER_TMP="$TMP/installer"
mkdir -p "$INSTALLER_TMP/bin"
cat >"$INSTALLER_TMP/os-release" <<'EOF'
ID=almalinux
ID_LIKE="rhel centos fedora"
PRETTY_NAME="AlmaLinux 9.8 (Olive Jaguar)"
VERSION="9.8 (Olive Jaguar)"
EOF
cat >"$INSTALLER_TMP/bin/id" <<'EOF'
#!/bin/sh
printf '0\n'
EOF
cat >"$INSTALLER_TMP/bin/uname" <<'EOF'
#!/bin/sh
case "$1" in
  -s) printf 'Linux\n' ;;
  -m) printf 'x86_64\n' ;;
esac
EOF
cat >"$INSTALLER_TMP/bin/curl" <<'EOF'
#!/bin/sh
for argument in "$@"; do
  case "$argument" in https://*) url="$argument" ;; esac
done
printf '%s\n' "$url" >>"${LWS_INSTALLER_TEST_LOG:?}"
while [ "$#" -gt 0 ]; do
  if [ "$1" = -o ]; then
    shift
    output="$1"
    break
  fi
  shift
done
case "$url" in
  *'/releases/latest') printf '{"browser_download_url":"https://example.test/lws.rpm"}\n' >"$output" ;;
  *) : >"$output" ;;
esac
EOF
cat >"$INSTALLER_TMP/bin/dnf" <<'EOF'
#!/bin/sh
printf '%s\n' "$*" >>"${LWS_INSTALLER_TEST_LOG:?}"
EOF
chmod +x "$INSTALLER_TMP/bin"/*
export LWS_INSTALLER_TEST_LOG="$INSTALLER_TMP/log"
PATH="$INSTALLER_TMP/bin:$PATH" LWS_OS_RELEASE_FILE="$INSTALLER_TMP/os-release" "$ROOT/scripts/install.sh" >/dev/null
grep -qx 'https://api.github.com/repos/LabWebSystem/Core/releases/latest' "$INSTALLER_TMP/log"
grep -qx 'install -y .*/lws.rpm' "$INSTALLER_TMP/log"
printf 'テスト: 成功\n'
