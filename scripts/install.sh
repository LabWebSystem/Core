#!/usr/bin/env bash
set -Eeuo pipefail
REPOSITORY="${LWS_REPOSITORY:-LabWebSystem/Core}"
VERSION="${LWS_VERSION:-latest}"
TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT
[[ "$(id -u)" -eq 0 ]] || { printf 'このインストーラーはroot権限で実行してください。\n' >&2; exit 1; }
[[ "$(uname -s)" == Linux ]] || { printf 'LWSが対応するOSはLinuxのみです。\n' >&2; exit 1; }
case "$(uname -m)" in x86_64|amd64) ARCH="amd64"; RPM_ARCH="x86_64" ;; aarch64|arm64) ARCH="arm64"; RPM_ARCH="aarch64" ;; *) printf '対応していないアーキテクチャです。\n' >&2; exit 1 ;; esac
OS_RELEASE_FILE="${LWS_OS_RELEASE_FILE:-/etc/os-release}"
[[ -r "$OS_RELEASE_FILE" ]] || { printf 'OS情報を読み取れません: %s\n' "$OS_RELEASE_FILE" >&2; exit 1; }
# /etc/os-releaseはOSが提供するキーと値だけを含む信頼できるファイルです。
# shellcheck disable=SC1090
. "$OS_RELEASE_FILE"
case " ${ID:-} ${ID_LIKE:-} " in
  *" ubuntu "*|*" debian "*) PACKAGE_KIND="deb"; PACKAGE_MANAGER="apt-get" ;;
  *" almalinux "*|*" rhel "*|*" fedora "*) PACKAGE_KIND="rpm"; PACKAGE_MANAGER="dnf" ;;
  *) printf '対応OSはUbuntu系またはAlmaLinux系です（検出値: %s）。\n' "${PRETTY_NAME:-不明}" >&2; exit 1 ;;
esac
command -v "$PACKAGE_MANAGER" >/dev/null 2>&1 || { printf '%sが見つかりません。\n' "$PACKAGE_MANAGER" >&2; exit 1; }
if [[ "$VERSION" == latest ]]; then API="https://api.github.com/repos/$REPOSITORY/releases/latest"; else API="https://api.github.com/repos/$REPOSITORY/releases/tags/lws-v$VERSION"; fi
command -v curl >/dev/null 2>&1 || { printf 'curlが必要です。\n' >&2; exit 1; }
JSON="$TMPDIR/release.json"; curl --fail --silent --show-error --location "$API" -o "$JSON"
if [[ "$PACKAGE_KIND" == deb ]]; then
  ASSET="$(awk -v arch="$ARCH" -F'"' '/browser_download_url/ && /\.deb"/ && $0 ~ arch {print $4; exit}' "$JSON")"
else
  ASSET="$(awk -F'"' '/browser_download_url/ && /\.rpm"/ {print $4; exit}' "$JSON")"
fi
[[ -n "$ASSET" ]] || { printf 'このアーキテクチャ向けのパッケージが見つかりません。\n' >&2; exit 1; }
FILE="$TMPDIR/$(basename "$ASSET")"; curl --fail --silent --show-error --location "$ASSET" -o "$FILE"
"$PACKAGE_MANAGER" install -y "$FILE"
printf 'LWSをインストールしました。実行例: sudo lwsctl start --domain example.internal\n'
