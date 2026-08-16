FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY backend ./backend
COPY version ./version
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/lws-backend ./backend/cmd/lws-backend

FROM alpine:3.22
RUN apk add --no-cache ca-certificates docker-cli docker-cli-compose git
COPY --from=build /out/lws-backend /usr/local/bin/lws-backend
EXPOSE 8080
ENTRYPOINT ["lws-backend"]
