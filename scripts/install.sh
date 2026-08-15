#!/usr/bin/env bash
set -Eeuo pipefail
REPOSITORY="${LWS_REPOSITORY:-labwebsystem/Core}"
VERSION="${LWS_VERSION:-latest}"
TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT
[[ "$(id -u)" -eq 0 ]] || { printf 'Run this installer as root.\n' >&2; exit 1; }
[[ "$(uname -s)" == Linux ]] || { printf 'LWS supports Linux only.\n' >&2; exit 1; }
case "$(uname -m)" in x86_64|amd64) ARCH="amd64"; RPM_ARCH="x86_64" ;; aarch64|arm64) ARCH="arm64"; RPM_ARCH="aarch64" ;; *) printf 'Unsupported architecture.\n' >&2; exit 1 ;; esac
if [[ "$VERSION" == latest ]]; then API="https://api.github.com/repos/$REPOSITORY/releases/latest"; else API="https://api.github.com/repos/$REPOSITORY/releases/tags/lws-v$VERSION"; fi
command -v curl >/dev/null 2>&1 || { printf 'curl is required.\n' >&2; exit 1; }
JSON="$TMPDIR/release.json"; curl --fail --silent --show-error --location "$API" -o "$JSON"
ASSET="$(awk -v arch="$ARCH" -F'"' '/browser_download_url/ && /\.deb"/ && $0 ~ arch {print $4; exit}' "$JSON")"
if [[ -z "$ASSET" ]]; then ASSET="$(awk -v arch="$RPM_ARCH" -F'"' '/browser_download_url/ && /\.rpm"/ && $0 ~ arch {print $4; exit}' "$JSON")"; fi
[[ -n "$ASSET" ]] || { printf 'No package asset found for this architecture.\n' >&2; exit 1; }
FILE="$TMPDIR/$(basename "$ASSET")"; curl --fail --silent --show-error --location "$ASSET" -o "$FILE"
case "$FILE" in
  *.deb) apt-get install -y "$FILE" ;;
  *.rpm) dnf install -y "$FILE" ;;
  *) printf 'Unknown package asset.\n' >&2; exit 1 ;;
esac
printf 'LWS installed. Run: sudo lwsctl start --domain example.internal\n'
