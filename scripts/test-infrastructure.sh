#!/usr/bin/env bash
set -Eeuo pipefail

readonly ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
readonly PROJECT="lws-it-$$"
TMP="$(mktemp -d)"

cleanup() {
  docker compose -p "$PROJECT" -f "$TMP/compose.yaml" down --volumes --remove-orphans >/dev/null 2>&1 || true
  docker volume rm "${PROJECT}_owned" "${PROJECT}_foreign" >/dev/null 2>&1 || true
  rm -rf "$TMP"
}
trap cleanup EXIT

mkdir -p "$TMP/generated"
cat >"$TMP/generated/Caddyfile" <<'EOF'
{
  auto_https off
}
http://app-a.example.test {
  reverse_proxy app-a:80
}
http://app-b.example.test {
  reverse_proxy app-b:80
}
EOF
cat >"$TMP/generated/hosts" <<'EOF'
127.0.0.1 app-a.example.test
127.0.0.1 app-b.example.test
EOF
cat >"$TMP/Corefile" <<'EOF'
.:53 {
  hosts /var/lib/lws/generated/hosts {
    fallthrough
  }
}
EOF
cat >"$TMP/compose.yaml" <<'EOF'
services:
  caddy:
    image: caddy:2.10-alpine
    command: caddy run --config /etc/caddy/Caddyfile --adapter caddyfile
    ports:
      - "18080:80"
    volumes:
      - ${TEST_ROOT}/generated:/etc/caddy:ro
    networks: [internal, edge-a, edge-b]
    labels:
      com.labwebsystem.owner: lws
      com.labwebsystem.installation-id: integration

  coredns:
    image: coredns/coredns:1.12.4
    command: -conf /etc/coredns/Corefile
    ports:
      - "18553:53/tcp"
      - "18553:53/udp"
    volumes:
      - ${TEST_ROOT}/Corefile:/etc/coredns/Corefile:ro
      - ${TEST_ROOT}/generated:/var/lib/lws/generated:ro
    networks: [internal]
    labels:
      com.labwebsystem.owner: lws
      com.labwebsystem.installation-id: integration

  app-a:
    image: nginx:1.27-alpine
    command: ["/bin/sh", "-c", "printf 'app-a\\n' > /usr/share/nginx/html/index.html && nginx -g 'daemon off;'"]
    networks: [edge-a]
    labels:
      com.labwebsystem.owner: lws
      com.labwebsystem.installation-id: integration
      com.labwebsystem.app-id: app-a

  app-b:
    image: nginx:1.27-alpine
    command: ["/bin/sh", "-c", "printf 'app-b\\n' > /usr/share/nginx/html/index.html && nginx -g 'daemon off;'"]
    networks: [edge-b]
    labels:
      com.labwebsystem.owner: lws
      com.labwebsystem.installation-id: integration
      com.labwebsystem.app-id: app-b

networks:
  internal:
  edge-a:
  edge-b:
EOF

export TEST_ROOT="$TMP"
docker compose -p "$PROJECT" -f "$TMP/compose.yaml" up -d --wait >/dev/null

wait_for_http() {
  local host="$1"
  local expected="$2"
  local response=""
  for _ in $(seq 1 20); do
    response="$(curl --silent --fail --header "Host: $host" http://127.0.0.1:18080/ 2>/dev/null || true)"
    if [[ "$response" == "$expected" ]]; then
      return 0
    fi
    sleep 1
  done
  docker logs "${PROJECT}-caddy-1" >&2
  return 1
}

wait_for_http app-a.example.test app-a
wait_for_http app-b.example.test app-b
app_a_response="app-a"
app_b_response="app-b"
test "$app_a_response" = "app-a"
test "$app_b_response" = "app-b"
test "$(dig +short @127.0.0.1 -p 18553 app-a.example.test)" = "127.0.0.1"

app_a_networks="$(docker inspect -f '{{json .NetworkSettings.Networks}}' "${PROJECT}-app-a-1")"
! grep -q 'edge-b' <<<"$app_a_networks"
app_b_networks="$(docker inspect -f '{{json .NetworkSettings.Networks}}' "${PROJECT}-app-b-1")"
! grep -q 'edge-a' <<<"$app_b_networks"

docker exec "${PROJECT}-caddy-1" caddy validate --config /etc/caddy/Caddyfile --adapter caddyfile >/dev/null
docker exec "${PROJECT}-caddy-1" caddy reload --config /etc/caddy/Caddyfile --adapter caddyfile --address localhost:2019 >/dev/null
docker kill --signal HUP "${PROJECT}-coredns-1" >/dev/null

cp "$TMP/generated/Caddyfile" "$TMP/old-Caddyfile"
cp "$TMP/generated/hosts" "$TMP/old-hosts"
printf 'invalid caddy config {\n' >"$TMP/generated/Caddyfile"
if docker exec "${PROJECT}-caddy-1" caddy validate --config /etc/caddy/Caddyfile --adapter caddyfile >/dev/null 2>&1; then
  printf '不正なCaddyfileを受理しました\n' >&2
  exit 1
fi
cp "$TMP/old-Caddyfile" "$TMP/generated/Caddyfile"
cp "$TMP/old-hosts" "$TMP/generated/hosts"
docker exec "${PROJECT}-caddy-1" caddy reload --config /etc/caddy/Caddyfile --adapter caddyfile --address localhost:2019 >/dev/null
docker kill --signal HUP "${PROJECT}-coredns-1" >/dev/null

docker volume create --label com.labwebsystem.owner=lws --label com.labwebsystem.installation-id=integration --label com.labwebsystem.app-id=app-a "${PROJECT}_owned" >/dev/null
docker volume create --label com.labwebsystem.owner=foreign "${PROJECT}_foreign" >/dev/null
docker volume rm "${PROJECT}_owned" >/dev/null
docker volume inspect "${PROJECT}_foreign" >/dev/null

printf 'Infrastructure統合テスト: 成功\n'
