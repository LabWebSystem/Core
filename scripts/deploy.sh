#!/usr/bin/env bash
set -Eeuo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VERSION=""
FORCE=0
while (($#)); do
  case "$1" in
    --version) VERSION="${2:-}"; shift 2 ;;
    --force) FORCE=1; shift ;;
    *) printf '使い方: mise run deploy --version x.y.z [--force]\n' >&2; exit 2 ;;
  esac
done
[[ "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || { printf 'バージョンはx.y.z形式で指定してください\n' >&2; exit 2; }
command -v gh >/dev/null 2>&1 || { printf 'デプロイにはgh CLIが必要です\n' >&2; exit 1; }
REPOSITORY="${LWS_REPOSITORY:-$(gh repo view --json nameWithOwner --jq .nameWithOwner)}"
TAG="lws-v$VERSION"
if gh release view "$TAG" --repo "$REPOSITORY" >/dev/null 2>&1; then
  if (( ! FORCE )); then
    read -r -p "リリース$TAGは存在します。置き換えますか? [y/N]: " answer
    [[ "$answer" =~ ^(y|Y|yes|YES)$ ]] || { printf 'デプロイをキャンセルしました\n'; exit 1; }
  fi
  gh release delete "$TAG" --repo "$REPOSITORY" --yes
fi
git -C "$ROOT" tag -f "$TAG"
git -C "$ROOT" push origin "+$TAG"
TAG_COMMIT="$(git -C "$ROOT" rev-parse "$TAG^{commit}")"
RUN_ID=""
for _ in {1..15}; do
  RUN_ID="$(gh run list --repo "$REPOSITORY" --workflow release-lws.yml --event push --commit "$TAG_COMMIT" --limit 1 --json databaseId --jq '.[0].databaseId')"
  [[ -n "$RUN_ID" ]] && break
  sleep 2
done
[[ -n "$RUN_ID" ]] || { printf 'リリースWorkflowを確認できません\n' >&2; exit 1; }
gh run watch "$RUN_ID" --repo "$REPOSITORY" --exit-status
