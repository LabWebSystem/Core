#!/usr/bin/env bash
set -Eeuo pipefail

readonly ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
readonly PROJECT="lws-it-$$"
readonly AVAHI_IMAGE="${PROJECT}-avahi:local"
TMP="$(mktemp -d)"

cleanup() {
  docker compose -p "$PROJECT" -f "$TMP/compose.yaml" down --volumes --remove-orphans >/dev/null 2>&1 || true
  docker volume rm "${PROJECT}_owned" "${PROJECT}_foreign" >/dev/null 2>&1 || true
  docker image rm "$AVAHI_IMAGE" >/dev/null 2>&1 || true
  rm -rf "$TMP"
}
trap cleanup EXIT

mkdir -p "$TMP/generated"
cat >"$TMP/generated/hosts" <<'EOF'
192.0.2.10 api-home.local
192.0.2.10 dashboard-home.local
192.0.2.10 app-a-home.local
EOF

docker build --network=host -f "$ROOT/infrastructure/Dockerfile.avahi" -t "$AVAHI_IMAGE" "$ROOT" >/dev/null
docker run --rm --entrypoint avahi-daemon "$AVAHI_IMAGE" --version >/dev/null
set +e
timeout --foreground 4s docker run --rm --network bridge --cap-add NET_ADMIN --cap-add NET_RAW \
  --volume "$TMP/generated/hosts:/var/lib/lws/generated/hosts:ro" "$AVAHI_IMAGE" \
  >"$TMP/avahi.log" 2>&1
avahi_status=$?
set -e
if [[ "$avahi_status" -ne 124 ]] || ! grep -q 'Server startup complete' "$TMP/avahi.log"; then
  cat "$TMP/avahi.log" >&2
  printf 'Avahiが隔離ネットワークで起動しませんでした\n' >&2
  exit 1
fi
if grep -Eq 'coredns|:53(/|:|\")' "$ROOT/infrastructure/compose.yaml"; then
  printf 'Composeに旧DNSサービスまたは53番ポートが残っています\n' >&2
  exit 1
fi
if grep -Eq '^  dashboard:' "$ROOT/infrastructure/compose.yaml"; then
  printf 'Dashboardが基盤Compose serviceとして残っています\n' >&2
  exit 1
fi

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
app_a_networks="$(docker inspect -f '{{json .NetworkSettings.Networks}}' "${PROJECT}-app-a-1")"
! grep -q 'edge-b' <<<"$app_a_networks"
app_b_networks="$(docker inspect -f '{{json .NetworkSettings.Networks}}' "${PROJECT}-app-b-1")"
! grep -q 'edge-a' <<<"$app_b_networks"

docker volume create --label com.labwebsystem.owner=lws --label com.labwebsystem.installation-id=integration --label com.labwebsystem.app-id=app-a "${PROJECT}_owned" >/dev/null
docker volume create --label com.labwebsystem.owner=foreign "${PROJECT}_foreign" >/dev/null
docker volume rm "${PROJECT}_owned" >/dev/null
docker volume inspect "${PROJECT}_foreign" >/dev/null
