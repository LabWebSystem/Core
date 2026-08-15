#!/usr/bin/env bash
set -Eeuo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VERSION=""
FORCE=0
while (($#)); do
  case "$1" in
    --version) VERSION="${2:-}"; shift 2 ;;
    --force) FORCE=1; shift ;;
    *) printf 'Usage: mise run deploy --version x.y.z [--force]\n' >&2; exit 2 ;;
  esac
done
[[ "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || { printf 'version must be x.y.z\n' >&2; exit 2; }
command -v gh >/dev/null 2>&1 || { printf 'gh CLI is required for deploy.\n' >&2; exit 1; }
REPOSITORY="${LWS_REPOSITORY:-$(gh repo view --json nameWithOwner --jq .nameWithOwner)}"
TAG="lws-v$VERSION"
if gh release view "$TAG" --repo "$REPOSITORY" >/dev/null 2>&1; then
  if (( ! FORCE )); then
    read -r -p "Release $TAG exists. Replace it? [y/N]: " answer
    [[ "$answer" =~ ^(y|Y|yes|YES)$ ]] || { printf 'deploy cancelled\n'; exit 1; }
  fi
  gh release delete "$TAG" --repo "$REPOSITORY" --yes
fi
"$ROOT/scripts/package.sh" --version "$VERSION"
git -C "$ROOT" tag -f "$TAG"
git -C "$ROOT" push origin "+$TAG"
gh release create "$TAG" --repo "$REPOSITORY" "$ROOT/dist"/* --title "LWS $VERSION" --generate-notes
