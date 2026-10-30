#!/usr/bin/env bash
set -Eeuo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
tmp="$(mktemp -d)"
installation="e2e-$(python3 -c 'import uuid; print(uuid.uuid4().hex)')"
caddy="lws-${installation}-caddy"
backend_pid=""

cleanup() {
  if [[ -n "$backend_pid" ]]; then
    kill "$backend_pid" 2>/dev/null || true
    wait "$backend_pid" 2>/dev/null || true
  fi
  while IFS= read -r id; do
    [[ -z "$id" ]] || docker rm -f "$id" >/dev/null
  done < <(docker ps -aq --filter "label=com.labwebsystem.owner=lws" --filter "label=com.labwebsystem.installation-id=$installation")
  while IFS= read -r id; do
    [[ -z "$id" ]] || docker network rm "$id" >/dev/null
  done < <(docker network ls -q --filter "label=com.labwebsystem.owner=lws" --filter "label=com.labwebsystem.installation-id=$installation")
  if [[ -f "$tmp/database.sqlite" ]]; then
    while IFS= read -r app_id; do
      [[ "$app_id" =~ ^[0-9a-f-]{36}$ ]] || continue
      project="lws-app-$app_id"
      while IFS= read -r id; do
        [[ -z "$id" ]] && continue
        actual="$(docker network inspect --format '{{index .Labels "com.docker.compose.project"}}' "$id")"
        [[ "$actual" == "$project" ]] && docker network rm "$id" >/dev/null
      done < <(docker network ls -q --filter "label=com.docker.compose.project=$project")
    done < <(python3 -c 'import sqlite3,sys; c=sqlite3.connect(sys.argv[1]); print("\n".join(r[0] for r in c.execute("select id from applications")))' "$tmp/database.sqlite")
  fi
  while IFS= read -r name; do
    [[ -z "$name" ]] || docker volume rm "$name" >/dev/null
  done < <(docker volume ls -q --filter "label=com.labwebsystem.owner=lws" --filter "label=com.labwebsystem.installation-id=$installation")
  rm -rf "$tmp"
}
trap cleanup EXIT

docker pull caddy:2.10-alpine >/dev/null
web_port="$(python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1",0)); print(s.getsockname()[1]); s.close()')"
docker run -d --name "$caddy" --label com.labwebsystem.owner=lws --label "com.labwebsystem.installation-id=$installation" caddy:2.10-alpine >/dev/null
go build -o "$tmp/backend" "$root/test/e2e/backend"
LWS_E2E_ROOT="$tmp" LWS_E2E_INSTALLATION="$installation" LWS_E2E_CADDY="$caddy" LWS_E2E_ORIGIN="http://127.0.0.1:$web_port" "$tmp/backend" >"$tmp/backend.log" 2>&1 &
backend_pid=$!
for _ in $(seq 1 100); do
  [[ -s "$tmp/backend-address" ]] && break
  if ! kill -0 "$backend_pid" 2>/dev/null; then cat "$tmp/backend.log" >&2; exit 1; fi
  sleep 0.1
done
if [[ ! -s "$tmp/backend-address" ]]; then
  printf 'テスト用Backendを起動できません。\n' >&2
  exit 1
fi
LWS_E2E_BACKEND="$(cat "$tmp/backend-address")" LWS_E2E_INSTALLATION="$installation" LWS_E2E_WEB_PORT="$web_port" npm --prefix "$root/dashboard" run test:e2e -- e2e/import-live.spec.ts
