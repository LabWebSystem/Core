# syntax=docker/dockerfile:1.7
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download
COPY backend ./backend
COPY version ./version
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/lws-backend ./backend/cmd/lws-backend

FROM alpine:3.22
ARG COMPOSE_VERSION=5.5.0
ARG TARGETARCH
RUN apk add --no-cache ca-certificates docker-cli eudev git wget \
    && case "${TARGETARCH}" in \
         amd64) compose_arch=x86_64; compose_sha256=c57ab918abd5b05ca7e7d0f275875dd1330a695074f309dc9eab1b49efafcd4b ;; \
         arm64) compose_arch=aarch64; compose_sha256=ff42489f5a9b879d5d117c5ffea6defc27390b3286da8ad52cbc9c6ab5df590e ;; \
         *) echo "未対応のComposeアーキテクチャです: ${TARGETARCH}" >&2; exit 1 ;; \
       esac \
    && mkdir -p /usr/local/lib/docker/cli-plugins \
    && wget -q "https://github.com/docker/compose/releases/download/v${COMPOSE_VERSION}/docker-compose-linux-${compose_arch}" -O /usr/local/lib/docker/cli-plugins/docker-compose \
    && echo "${compose_sha256}  /usr/local/lib/docker/cli-plugins/docker-compose" | sha256sum -c - \
    && chmod 0755 /usr/local/lib/docker/cli-plugins/docker-compose \
    && apk del wget
COPY --from=build /out/lws-backend /usr/local/bin/lws-backend
EXPOSE 8080
ENTRYPOINT ["lws-backend"]
