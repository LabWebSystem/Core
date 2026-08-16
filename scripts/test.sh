#!/usr/bin/env bash
set -Eeuo pipefail

readonly ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

TMP=""

cleanup() {
  [[ -z "$TMP" ]] || rm -rf "$TMP"
}

setup_cli_environment() {
  mkdir -p "$TMP/bin"

  cat >"$TMP/bin/docker" <<'EOF'
#!/bin/sh
case "$*" in
  *'ps --status running --services'*)
    if [ -n "${LWS_TEST_RUNNING_SERVICES:-}" ]; then
      printf '%s\n' "$LWS_TEST_RUNNING_SERVICES"
    fi
    ;;
esac
printf '%s | LWS_BASE_DOMAIN=%s LWS_VERSION=%s\n' \
  "$*" \
  "${LWS_BASE_DOMAIN:-}" \
  "${LWS_VERSION:-}" \
  >>"${LWS_TEST_LOG:?}"
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

  export PATH="$TMP/bin:$PATH"
  export LWS_TEST_LOG="$TMP/docker.log"
  export LWS_CONFIG_DIR="$TMP/etc"
  export LWS_STATE_DIR="$TMP/state"
  export LWS_COMPOSE_FILE="$ROOT/infrastructure/compose.yaml"
  export LWS_VERSION_FILE="$TMP/version"
  export LWS_INSTALLER_PATH="$TMP/update-installer"

  LWSCTL="$TMP/lwsctl"

  "$ROOT/scripts/build-lwsctl.sh" \
    --output "$LWSCTL"
}

test_help() {
  "$LWSCTL" >"$TMP/help" 2>&1 || test "$?" -eq 2

  grep -q \
    'LWSのライフサイクルをDocker Composeで管理します。' \
    "$TMP/help"

  grep -q \
    -- '-d, --domain ドメイン' \
    "$TMP/help"

  grep -q \
    -- '--purge' \
    "$TMP/help"

  grep -q \
    'down       LWS管理下の実行環境を停止して削除します。' \
    "$TMP/help"

  ! "$LWSCTL" uninstall >"$TMP/uninstall" 2>&1

  "$LWSCTL" start --help >"$TMP/start-help"

  grep -q \
    '設定済みのドメインを変更すると、DNSとReverse Proxyの設定を再生成します。' \
    "$TMP/start-help"
}

test_lifecycle() {
  "$LWSCTL" status >"$TMP/status-before"

  grep -q \
    '設定済み: NO' \
    "$TMP/status-before"

  grep -q \
    '先にlwsctl startを実行してください' \
    "$TMP/status-before"

  test "$(wc -l <"$TMP/status-before")" -eq 2
  test ! -s "$TMP/docker.log"

  "$LWSCTL" start \
    --domain example.internal

  grep -qx \
    'LWS_BASE_DOMAIN=example.internal' \
    "$TMP/etc/config.env"

  grep -qF \
    'up -d --remove-orphans | LWS_BASE_DOMAIN=example.internal LWS_VERSION=0.1.2' \
    "$TMP/docker.log"

  "$LWSCTL" stop

  grep -qF \
    'stop' \
    "$TMP/docker.log"
}

test_domain_change() {
  ! printf 'n\n' |
    "$LWSCTL" start \
      --domain changed.internal \
      >/dev/null 2>&1

  grep -qx \
    'LWS_BASE_DOMAIN=example.internal' \
    "$TMP/etc/config.env"

  "$LWSCTL" start \
    --domain changed.internal \
    --force

  grep -qx \
    'LWS_BASE_DOMAIN=changed.internal' \
    "$TMP/etc/config.env"
}

test_update() {
	: >"$TMP/docker.log"

  "$LWSCTL" update

  grep -qx \
    'package-update' \
    "$TMP/docker.log"

  grep -qx \
    'LWS_VERSION=0.2.0' \
    "$TMP/etc/config.env"

  grep -qF \
    'pull | LWS_BASE_DOMAIN=changed.internal LWS_VERSION=0.2.0' \
    "$TMP/docker.log"

  ! grep -qF \
    'up -d --remove-orphans' \
    "$TMP/docker.log"
}

test_update_running() {
  : >"$TMP/docker.log"
  export LWS_TEST_RUNNING_SERVICES=backend

  "$LWSCTL" update

  unset LWS_TEST_RUNNING_SERVICES

  grep -qF \
    'up -d --remove-orphans | LWS_BASE_DOMAIN=changed.internal LWS_VERSION=0.2.0' \
    "$TMP/docker.log"
}

test_down() {
  "$LWSCTL" down >"$TMP/down"

  grep -q \
    'LWSの実行環境を削除しました' \
    "$TMP/down"

  grep -qF \
    'down --remove-orphans | LWS_BASE_DOMAIN=changed.internal LWS_VERSION=0.2.0' \
    "$TMP/docker.log"

  test -f "$TMP/etc/config.env"

  "$LWSCTL" down \
    --purge \
    --force

  test ! -e "$TMP/etc"
  test ! -e "$TMP/state"
}

test_installer_almalinux() {
  local asset_name="$1"
  local installer_tmp="$TMP/installer"
  local fake_bin="$installer_tmp/bin"
  local os_release="$installer_tmp/os-release"
  local dnf_log="$installer_tmp/dnf.log"

  mkdir -p "$fake_bin"

  cat >"$os_release" <<'EOF'
ID="almalinux"
ID_LIKE="rhel centos fedora"
PRETTY_NAME="AlmaLinux"
EOF

  cat >"$fake_bin/id" <<'EOF'
#!/bin/sh
if [ "${1:-}" = "-u" ]; then
  printf '0\n'
  exit 0
fi

exec /usr/bin/id "$@"
EOF

  cat >"$fake_bin/uname" <<'EOF'
#!/bin/sh
case "${1:-}" in
  -s)
    printf 'Linux\n'
    ;;
  -m)
    printf 'x86_64\n'
    ;;
  *)
    exec /usr/bin/uname "$@"
    ;;
esac
EOF

  cat >"$fake_bin/curl" <<'EOF'
#!/bin/sh
output=""

while [ "$#" -gt 0 ]; do
  case "$1" in
    -o)
      output="$2"
      shift 2
      ;;
    *)
      shift
      ;;
  esac
done

case "$output" in
  */release.json)
    printf \
      '{\n  "assets": [\n    {\n      "browser_download_url": "https://example.invalid/%s"\n    }\n  ]\n}\n' \
      "${LWS_TEST_RPM_ASSET_NAME:?}" \
      >"$output"
    ;;
  *)
    printf 'fake-rpm\n' >"$output"
    ;;
esac
EOF

  cat >"$fake_bin/dnf" <<EOF
#!/bin/sh
printf '%s\n' "\$*" >>"$dnf_log"
EOF

  chmod +x \
    "$fake_bin/id" \
    "$fake_bin/uname" \
    "$fake_bin/curl" \
    "$fake_bin/dnf"

  PATH="$fake_bin:$PATH" \
  LWS_OS_RELEASE_FILE="$os_release" \
  LWS_TEST_RPM_ASSET_NAME="$asset_name" \
    "$ROOT/scripts/install.sh"

  grep -q \
    "install -y .*${asset_name}" \
    "$dnf_log"
}

test_release_selected_components() {
  local deploy_tmp="$TMP/deploy"
  local fake_bin="$deploy_tmp/bin"
  local deploy_log="$deploy_tmp/deploy.log"

  mkdir -p "$fake_bin"

  cat >"$fake_bin/git" <<'EOF'
#!/bin/sh
printf 'git %s\n' "$*" >>"${LWS_DEPLOY_TEST_LOG:?}"

case "$*" in
  *'branch --show-current')
    printf 'main\n'
    ;;
  *'ls-remote --exit-code --tags'*)
    exit 2
    ;;
  *'rev-parse lws-v0.1.0^{commit}')
    printf 'lws-commit\n'
    ;;
  *'rev-parse sdk-v0.1.0^{commit}')
    printf 'sdk-commit\n'
    ;;
esac
EOF

  cat >"$fake_bin/gh" <<'EOF'
#!/bin/sh
printf 'gh %s\n' "$*" >>"${LWS_DEPLOY_TEST_LOG:?}"

case "$1 $2" in
  'repo view')
    printf 'labwebsystem/core\n'
    ;;
  'release view')
    exit 1
    ;;
  'run list')
    printf '100\n'
    ;;
  'run watch')
    printf 'Workflowの監視を完了しました\n'
    ;;
esac
EOF

  chmod +x "$fake_bin/git" "$fake_bin/gh"

  PATH="$fake_bin:$PATH" \
  LWS_DEPLOY_TEST_LOG="$deploy_log" \
    "$ROOT/scripts/release.sh" core sdk

  local all_output="$deploy_tmp/release-all"

  PATH="$fake_bin:$PATH" \
  LWS_DEPLOY_TEST_LOG="$deploy_log" \
    "$ROOT/scripts/release.sh" all >"$all_output" 2>&1

  grep -qF 'push origin +lws-v0.1.0' "$deploy_log"
  grep -qF 'push origin +sdk-v0.1.0' "$deploy_log"
  grep -qF 'gh run list --repo labwebsystem/core --workflow release-lws.yml' "$deploy_log"
  grep -qF 'gh run list --repo labwebsystem/core --workflow release-sdk.yml' "$deploy_log"
  test "$(grep -cF 'gh run watch 100 --repo labwebsystem/core --exit-status --compact' "$deploy_log")" -eq 4
  grep -qF '[LWS] Workflowの監視を完了しました' "$all_output"
  grep -qF '[SDK] Workflowの監視を完了しました' "$all_output"
}

test_version_sources() {
  local versions

  mise run version core 0.1.0
  versions="$(mise run version)"

  grep -qx 'core 0.1.0' <<<"$versions"
  grep -qx 'sdk 0.1.0' <<<"$versions"
  test "$("$ROOT/scripts/version.sh" core)" = '0.1.0'
  test "$("$ROOT/scripts/version.sh" sdk)" = '0.1.0'
}

main() {
  TMP="$(mktemp -d)"
  trap cleanup EXIT

  setup_cli_environment

  test_help
  test_lifecycle
  test_domain_change
  test_update
  test_update_running
  test_down
  test_installer_almalinux 'lws-0.1.0.x86_64.rpm'
  test_installer_almalinux 'lws-0.1.0.rpm'
  test_version_sources
  test_release_selected_components

  printf 'テスト: 成功\n'
}

main "$@"
