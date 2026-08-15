#!/usr/bin/env bash
set -Eeuo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP="$(mktemp -d)"; trap 'rm -rf "$TMP"' EXIT
mkdir -p "$TMP/bin"
cat >"$TMP/bin/docker" <<'EOF'
#!/bin/sh
printf '%s | LWS_BASE_DOMAIN=%s LWS_VERSION=%s\n' "$*" "${LWS_BASE_DOMAIN:-}" "${LWS_VERSION:-}" >>"${LWS_TEST_LOG:?}"
EOF
chmod +x "$TMP/bin/docker"
printf '0.1.2\n' >"$TMP/version"
cat >"$TMP/update-installer" <<'EOF'
#!/bin/sh
test -z "${LWS_VERSION+x}"
printf '0.2.0\n' >"${LWS_VERSION_FILE:?}"
printf 'package-update\n' >>"${LWS_TEST_LOG:?}"
EOF
chmod +x "$TMP/update-installer"
export PATH="$TMP/bin:$PATH" LWS_TEST_LOG="$TMP/docker.log" LWS_CONFIG_DIR="$TMP/etc" LWS_STATE_DIR="$TMP/state" LWS_COMPOSE_FILE="$ROOT/infrastructure/compose.yaml" LWS_VERSION_FILE="$TMP/version" LWS_INSTALLER_PATH="$TMP/update-installer" LWS_SKIP_PACKAGE_REMOVE=1
LWSCTL="$ROOT/scripts/lwsctl"

"$LWSCTL" status >"$TMP/status-before"
grep -q '設定済み: NO' "$TMP/status-before"
"$ROOT/scripts/lwsctl" start --domain example.internal
grep -qx 'LWS_BASE_DOMAIN=example.internal' "$TMP/etc/config.env"
grep -qF 'compose --project-name lws --file' "$TMP/docker.log"
grep -qF 'up -d --remove-orphans | LWS_BASE_DOMAIN=example.internal LWS_VERSION=0.1.2' "$TMP/docker.log"

"$LWSCTL" status >"$TMP/status-after-start"
grep -q '設定済み: YES' "$TMP/status-after-start"
grep -q 'ドメイン: example.internal' "$TMP/status-after-start"
grep -qF 'ps | LWS_BASE_DOMAIN=example.internal LWS_VERSION=0.1.2' "$TMP/docker.log"

! printf 'n\n' | "$LWSCTL" start --domain changed.internal >/dev/null 2>&1
grep -qx 'LWS_BASE_DOMAIN=example.internal' "$TMP/etc/config.env"
"$LWSCTL" start --domain changed.internal --force
grep -qx 'LWS_BASE_DOMAIN=changed.internal' "$TMP/etc/config.env"
grep -qF 'up -d --remove-orphans | LWS_BASE_DOMAIN=changed.internal LWS_VERSION=0.1.2' "$TMP/docker.log"

"$LWSCTL" rebuild
grep -qF 'config | LWS_BASE_DOMAIN=changed.internal LWS_VERSION=0.1.2' "$TMP/docker.log"
grep -qF 'up -d --force-recreate --remove-orphans | LWS_BASE_DOMAIN=changed.internal LWS_VERSION=0.1.2' "$TMP/docker.log"
"$LWSCTL" update
grep -qx 'package-update' "$TMP/docker.log"
grep -qx 'LWS_VERSION=0.2.0' "$TMP/etc/config.env"
grep -qF 'pull | LWS_BASE_DOMAIN=changed.internal LWS_VERSION=0.2.0' "$TMP/docker.log"

"$LWSCTL" stop
grep -qF 'down --remove-orphans | LWS_BASE_DOMAIN=changed.internal LWS_VERSION=0.2.0' "$TMP/docker.log"
"$LWSCTL" uninstall >"$TMP/uninstall"
grep -q 'パッケージマネージャーによる削除をスキップしました' "$TMP/uninstall"
test -f "$TMP/etc/config.env"
"$LWSCTL" uninstall --purge --force
test ! -e "$TMP/etc"
test ! -e "$TMP/state"

! "$LWSCTL" start --domain bad_domain >/dev/null 2>&1

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
