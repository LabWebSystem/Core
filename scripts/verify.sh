#!/usr/bin/env bash
set -Eeuo pipefail

readonly ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
readonly WORKFLOW='quality.yml'
PROFILE="${1:-fast}"

usage() {
  printf '使い方: mise run verify [fast|full|qa|release]\n' >&2
}

die() {
  printf '%s\n' "$*" >&2
  exit 1
}

if (($# > 1)); then
  usage
  exit 2
fi

case "$PROFILE" in
  fast)
    ;;
  full|qa|release)
    PROFILE='full'
    ;;
  -h|--help)
    usage
    exit 0
    ;;
  *)
    usage
    die "未対応の検証範囲です: $PROFILE"
    ;;
esac

command -v git >/dev/null 2>&1 || die 'gitが必要です'
command -v gh >/dev/null 2>&1 || die 'GitHub CLI（gh）が必要です'
command -v python3 >/dev/null 2>&1 || die 'Python 3が必要です'

gh auth status >/dev/null 2>&1 || die 'GitHub CLIで認証してください: gh auth login'

if [[ -n "$(git -C "$ROOT" status --porcelain --untracked-files=all)" ]]; then
  die '未コミットの変更があるためActionsを起動できません。変更をcommitしてください'
fi

branch="$(git -C "$ROOT" branch --show-current)"
[[ -n "$branch" ]] || die 'detached HEADでは実行できません。push済みブランチへ切り替えてください'
head_sha="$(git -C "$ROOT" rev-parse HEAD)"
upstream="$(git -C "$ROOT" rev-parse --abbrev-ref --symbolic-full-name '@{upstream}' 2>/dev/null || true)"
[[ -n "$upstream" ]] || die '追跡先ブランチがありません。GitHubへブランチをpushしてください'
remote_name="${upstream%%/*}"
remote_branch="${upstream#*/}"
remote_line="$(git -C "$ROOT" ls-remote --heads "$remote_name" "refs/heads/$remote_branch")" || die 'GitHubの追跡先を確認できませんでした'
remote_sha="${remote_line%%$'\t'*}"
[[ "$remote_sha" == "$head_sha" ]] || die 'ローカルHEADが追跡先と一致しません。最新のコミットをGitHubへpushしてください'

repository="$(gh repo view --json nameWithOwner --jq '.nameWithOwner')"
before_ids="$(gh run list \
  --repo "$repository" \
  --workflow "$WORKFLOW" \
  --event workflow_dispatch \
  --branch "$branch" \
  --limit 100 \
  --json databaseId \
  --jq '[.[].databaseId]')"
request_id="verify-$(python3 -c 'import uuid; print(uuid.uuid4().hex)')"

printf 'GitHub Actionsで%s品質検証を開始します: %s @ %s\n' "$PROFILE" "$branch" "${head_sha:0:12}"
gh workflow run "$WORKFLOW" \
  --repo "$repository" \
  --ref "$branch" \
  --field "profile=$PROFILE" \
  --field "request_id=$request_id"

run_json=''
run_id=''
for _ in $(seq 1 30); do
  run_json="$(gh run list \
    --repo "$repository" \
    --workflow "$WORKFLOW" \
    --event workflow_dispatch \
    --branch "$branch" \
    --limit 100 \
    --json databaseId,headSha,createdAt,url,displayTitle)"
  run_id="$(BEFORE_IDS="$before_ids" RUNS_JSON="$run_json" HEAD_SHA="$head_sha" REQUEST_ID="$request_id" python3 - <<'PY'
import json
import os

before = set(json.loads(os.environ["BEFORE_IDS"]))
runs = json.loads(os.environ["RUNS_JSON"])
sha = os.environ["HEAD_SHA"]
request_id = os.environ["REQUEST_ID"]
matches = [run for run in runs if run["headSha"] == sha and run["databaseId"] not in before and request_id in run["displayTitle"]]
matches.sort(key=lambda run: run["createdAt"])
print(matches[-1]["databaseId"] if matches else "")
PY
  )"
  [[ -z "$run_id" ]] || break
  sleep 2
done

[[ -n "$run_id" ]] || die 'Workflow runを確認できませんでした。GitHub Actionsの画面を確認してください'
run_url="$(gh run view "$run_id" --repo "$repository" --json url --jq '.url')"
printf '実行URL: %s\n' "$run_url"

watch_status=0
gh run watch "$run_id" --repo "$repository" --exit-status --compact || watch_status=$?

artifact_dir="$ROOT/test/result/actions/$run_id"
mkdir -p "$artifact_dir"
if gh run download "$run_id" --repo "$repository" --dir "$artifact_dir"; then
  printf '検証artifact: %s\n' "$artifact_dir"
else
  printf '検証artifactを取得できませんでした。実行URLから確認してください: %s\n' "$run_url" >&2
  [[ "$watch_status" != 0 ]] || watch_status=1
fi

exit "$watch_status"
