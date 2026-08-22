#!/usr/bin/env bash
set -Eeuo pipefail

readonly ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
readonly HOST_PATH="$PATH"

TMP=""

cleanup() {
  [[ -z "$TMP" ]] || rm -rf "$TMP"
}

setup_cli_environment() {
  mkdir -p "$TMP/bin"

  cat >"$TMP/bin/docker" <<'EOF'
#!/bin/sh
case "$*" in
  *'ps -q --filter label=com.labwebsystem.owner=lws --filter label=com.labwebsystem.installation-id='*)
    printf 'owned-system-container\n'
    ;;
  *'ps -aq --filter label=com.labwebsystem.owner=lws --filter label=com.labwebsystem.installation-id='*'--filter label=com.labwebsystem.app-id'*)
    printf 'owned-app-container\n'
    ;;
  *'network ls -q --filter label=com.labwebsystem.owner=lws --filter label=com.labwebsystem.installation-id='*'--filter label=com.labwebsystem.app-id'*)
    printf 'owned-app-network\n'
    ;;
  *'volume ls -q --filter label=com.labwebsystem.owner=lws --filter label=com.labwebsystem.installation-id='*'--filter label=com.labwebsystem.app-id'*)
    printf 'owned-app-volume\n'
    ;;
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

  # CI runner上のDNS resolverなどに53番ポートを占有されていても、
  # CLIのライフサイクルテストは「空いているホスト」を決定的に再現する。
  cat >"$TMP/bin/ss" <<'EOF'
#!/bin/sh
exit 0
EOF
  chmod +x "$TMP/bin/ss"

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

test_update_before_start() {
  : >"$TMP/docker.log"

  "$LWSCTL" update

  grep -qx \
    'package-update' \
    "$TMP/docker.log"

  grep -qF \
    'pull | LWS_BASE_DOMAIN= LWS_VERSION=0.2.0' \
    "$TMP/docker.log"

  ! grep -qF \
    'up -d --remove-orphans' \
    "$TMP/docker.log"

  test ! -e "$TMP/etc/config.env"

  printf '0.1.2\n' >"$TMP/version"
  : >"$TMP/docker.log"
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

  grep -Eq '^LWS_INSTALLATION_ID=[0-9a-f-]{36}$' "$TMP/etc/config.env"
  grep -Eq '^LWS_PUBLIC_ADDRESS=[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$' "$TMP/etc/config.env"

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

test_config_migration() {
  printf 'LWS_BASE_DOMAIN=changed.internal\nLWS_VERSION=0.1.2\n' >"$TMP/etc/config.env"
  LWS_PUBLIC_ADDRESS=192.0.2.10 "$LWSCTL" status >"$TMP/status-migrated"
  grep -Eq '^LWS_INSTALLATION_ID=[0-9a-f-]{36}$' "$TMP/etc/config.env"
  grep -qx 'LWS_PUBLIC_ADDRESS=192.0.2.10' "$TMP/etc/config.env"
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

  grep -qF \
    'down --remove-orphans --volumes | LWS_BASE_DOMAIN=changed.internal LWS_VERSION=0.2.0' \
    "$TMP/docker.log"

  stop_line="$(grep -nF 'stop owned-system-container' "$TMP/docker.log" | cut -d: -f1)"
  down_line="$(grep -nF 'compose --project-name lws --file' "$TMP/docker.log" | tail -1 | cut -d: -f1)"
  app_remove_line="$(grep -nF 'rm -f owned-app-container' "$TMP/docker.log" | cut -d: -f1)"
  network_remove_line="$(grep -nF 'network rm owned-app-network' "$TMP/docker.log" | cut -d: -f1)"
  volume_remove_line="$(grep -nF 'volume rm owned-app-volume' "$TMP/docker.log" | cut -d: -f1)"
  test "$stop_line" -lt "$down_line"
  test "$down_line" -lt "$app_remove_line"
  test "$app_remove_line" -lt "$network_remove_line"
  test "$network_remove_line" -lt "$volume_remove_line"

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

test_package_upgrade_config_hook() {
  grep -qF 'chmod 0600 /etc/lws/config.env' "$ROOT/packaging/lws.preinst"
  grep -qF 'chmod 0600 /etc/lws/config.env' "$ROOT/packaging/lws.spec.in"
  grep -qF 'lws.preinst' "$ROOT/scripts/package.sh"
}

test_release_selected_components() {
  local deploy_tmp="$TMP/deploy"
  local fake_bin="$deploy_tmp/bin"
  local deploy_log="$deploy_tmp/deploy.log"
  local core_version sdk_version

  core_version="$("$ROOT/scripts/version.sh" core)"
  sdk_version="$("$ROOT/scripts/version.sh" sdk)"

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
  *"rev-parse lws-v${LWS_TEST_CORE_VERSION:?}^{commit}")
    printf 'lws-commit\n'
    ;;
  *"rev-parse sdk-v${LWS_TEST_SDK_VERSION:?}^{commit}")
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
  LWS_TEST_CORE_VERSION="$core_version" \
  LWS_TEST_SDK_VERSION="$sdk_version" \
    "$ROOT/scripts/release.sh" core sdk

  local all_output="$deploy_tmp/release-all"

  PATH="$fake_bin:$PATH" \
  LWS_DEPLOY_TEST_LOG="$deploy_log" \
  LWS_TEST_CORE_VERSION="$core_version" \
  LWS_TEST_SDK_VERSION="$sdk_version" \
    "$ROOT/scripts/release.sh" all >"$all_output" 2>&1

  grep -qF "push origin +lws-v$core_version" "$deploy_log"
  grep -qF "push origin +sdk-v$sdk_version" "$deploy_log"
  grep -qF 'gh run list --repo labwebsystem/core --workflow release-lws.yml' "$deploy_log"
  grep -qF 'gh run list --repo labwebsystem/core --workflow release-sdk.yml' "$deploy_log"
  test "$(grep -cF 'gh run watch 100 --repo labwebsystem/core --exit-status --compact' "$deploy_log")" -eq 4
  grep -qF '[LWS] Workflowの監視を完了しました' "$all_output"
  grep -qF '[SDK] Workflowの監視を完了しました' "$all_output"
}

test_version_sources() {
  local core_version sdk_version versions

  core_version="$("$ROOT/scripts/version.sh" core)"
  sdk_version="$("$ROOT/scripts/version.sh" sdk)"
  versions="$(mise run version)"

  grep -qx "core $core_version" <<<"$versions"
  grep -qx "sdk $sdk_version" <<<"$versions"
}

main() {
  local target="${1:-all}"

  case "$target" in
    all|fast|fixtures|cli|installer|release|backend|backend-http|backend-docker|infrastructure|architecture) ;;
    *)
      printf '不明なテスト対象です: %s\n' "$target" >&2
      return 2
      ;;
  esac

  TMP="$(mktemp -d)"
  trap cleanup EXIT

  if [[ "$target" == all || "$target" == fast || "$target" == fixtures ]]; then
    "$ROOT/scripts/test-fixtures.sh"
  fi
  if [[ "$target" == all || "$target" == fast || "$target" == architecture ]]; then
    "$ROOT/scripts/test-quality-architecture.sh"
  fi
  if [[ "$target" == all || "$target" == cli ]]; then
    setup_cli_environment
    test_help
    test_update_before_start
    test_lifecycle
    test_domain_change
    test_config_migration
    test_update
    test_update_running
    test_down
    test_version_sources
  fi
  if [[ "$target" == all || "$target" == installer ]]; then
    test_package_upgrade_config_hook
    test_installer_almalinux 'lws-0.1.0.x86_64.rpm'
    test_installer_almalinux 'lws-0.1.0.rpm'
  fi
  if [[ "$target" == all || "$target" == release ]]; then
    test_release_selected_components
  fi

  if [[ "$target" == all || "$target" == fast || "$target" == backend ]]; then
    (cd "$ROOT" && go test -count=1 ./backend/...)
  fi
  if [[ "$target" == all || "$target" == backend-http ]]; then
    (cd "$ROOT" && mise run check-openapi)
    (cd "$ROOT" && go test -list 'TestHTTP' ./backend/... | grep -q '^TestHTTP') || { printf 'backend-http対象テストがありません\n' >&2; return 1; }
    (cd "$ROOT" && go test -count=1 ./backend/... -run '^TestHTTP')
  fi
  if [[ "$target" == all || "$target" == backend-docker ]]; then
    (cd "$ROOT" && go test -list 'Test(Runtime|OSRunner|Compose|Docker|Ownership)' ./backend/... | grep -Eq '^Test(Runtime|OSRunner|Compose|Docker|Ownership)') || { printf 'backend-docker対象テストがありません\n' >&2; return 1; }
    (cd "$ROOT" && go test -count=1 ./backend/... -run '^Test(Runtime|OSRunner|Compose|Docker|Ownership)')
  fi
  if [[ "$target" == all || "$target" == infrastructure ]]; then
    (cd "$ROOT" && go test -list '^TestDerived' ./backend/... | grep -q '^TestDerived') || { printf 'infrastructure対象テストがありません\n' >&2; return 1; }
    (cd "$ROOT" && go test -count=1 ./backend/... -run '^TestDerived')
    PATH="$HOST_PATH" "$ROOT/scripts/test-infrastructure.sh"
  fi

  printf 'テスト: 成功\n'
}

main "$@"
