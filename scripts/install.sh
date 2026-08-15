#!/usr/bin/env bash
set -Eeuo pipefail

readonly REPOSITORY="${LWS_REPOSITORY:-LabWebSystem/Core}"
readonly RELEASE_VERSION="${LWS_VERSION:-latest}"
readonly OS_RELEASE_FILE="${LWS_OS_RELEASE_FILE:-/etc/os-release}"

TMPDIR=""

die() {
  printf '%s\n' "$*" >&2
  exit 1
}

require_command() {
  command -v "$1" >/dev/null 2>&1 ||
    die "$1が見つかりません"
}

cleanup() {
  [[ -z "$TMPDIR" ]] || rm -rf "$TMPDIR"
}

download() {
  local url="$1"
  local output="$2"

  curl \
    --http1.1 \
    --fail \
    --silent \
    --show-error \
    --location \
    "$url" \
    -o "$output"
}

detect_architecture() {
  case "$(uname -m)" in
    x86_64 | amd64)
      ARCH="amd64"
      RPM_ARCH="x86_64"
      ;;
    aarch64 | arm64)
      ARCH="arm64"
      RPM_ARCH="aarch64"
      ;;
    *)
      die '対応していないアーキテクチャです'
      ;;
  esac
}

detect_distribution() {
  [[ -r "$OS_RELEASE_FILE" ]] ||
    die "OS情報を読み取れません: $OS_RELEASE_FILE"

  # /etc/os-releaseはOSが提供するキーと値だけを含むファイルです。
  # shellcheck disable=SC1090
  . "$OS_RELEASE_FILE"

  case " ${ID:-} ${ID_LIKE:-} " in
    *" ubuntu "* | *" debian "*)
      PACKAGE_KIND="deb"
      PACKAGE_MANAGER="apt-get"
      ;;
    *" almalinux "* | *" rhel "* | *" fedora "*)
      PACKAGE_KIND="rpm"
      PACKAGE_MANAGER="dnf"
      ;;
    *)
      die "対応OSはUbuntu系またはAlmaLinux系です（検出値: ${PRETTY_NAME:-不明}）"
      ;;
  esac
}

release_api_url() {
  if [[ "$RELEASE_VERSION" == "latest" ]]; then
    printf 'https://api.github.com/repos/%s/releases/latest\n' "$REPOSITORY"
    return
  fi

  printf \
    'https://api.github.com/repos/%s/releases/tags/lws-v%s\n' \
    "$REPOSITORY" \
    "$RELEASE_VERSION"
}

find_package_asset() {
  local json="$1"

  case "$PACKAGE_KIND" in
    deb)
      awk \
        -v arch="$ARCH" \
        -F'"' \
        '/browser_download_url/ && /\.deb"/ && $0 ~ arch { print $4; exit }' \
        "$json"
      ;;
    rpm)
      awk \
        -v arch="$RPM_ARCH" \
        -F'"' \
        '/browser_download_url/ && /\.rpm"/ && $0 ~ arch { print $4; exit }' \
        "$json"
      ;;
  esac
}

main() {
  [[ "$(id -u)" -eq 0 ]] ||
    die 'このインストーラーはroot権限で実行してください'

  [[ "$(uname -s)" == "Linux" ]] ||
    die 'LWSが対応するOSはLinuxのみです'

  require_command curl

  detect_architecture
  detect_distribution
  require_command "$PACKAGE_MANAGER"

  TMPDIR="$(mktemp -d)"
  trap cleanup EXIT

  local release_json="$TMPDIR/release.json"
  local api
  local asset
  local package_file

  api="$(release_api_url)"
  download "$api" "$release_json"

  asset="$(find_package_asset "$release_json")"
  [[ -n "$asset" ]] ||
    die 'このアーキテクチャ向けのパッケージが見つかりません'

  package_file="$TMPDIR/$(basename "$asset")"
  download "$asset" "$package_file"

  "$PACKAGE_MANAGER" install -y "$package_file"

  printf 'LWSをインストールしました。実行例: sudo lwsctl start --domain example.internal\n'
}

main "$@"