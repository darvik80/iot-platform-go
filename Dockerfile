# syntax=docker/dockerfile:1
FROM node:24-bookworm AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.27-bookworm AS build
ARG VERSION=dev
ARG COMMIT=unknown
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/internal/webui/dist ./internal/webui/dist
RUN test -f internal/webui/dist/index.html \
 && CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w -X main.Version=${VERSION} -X main.Commit=${COMMIT}" -o /out/platform ./cmd/platform

FROM alpine:3.24
# /bin/sh is busybox ash: no brace expansion, list directories explicitly.
RUN apk add --no-cache ca-certificates tzdata su-exec \
  && adduser -D -u 1000 platform \
  && mkdir -p /opt/iot-platform/bin /opt/iot-platform/etc/certs /opt/iot-platform/lib \
  && chown -R platform:platform /opt/iot-platform/etc /opt/iot-platform/lib
WORKDIR /opt/iot-platform
COPY --from=build /out/platform /opt/iot-platform/bin/platform
COPY docker-entrypoint.sh /docker-entrypoint.sh
# Strip CR in case the build context comes from a Windows checkout (core.autocrlf).
RUN sed -i 's/\r$//' /docker-entrypoint.sh && chmod +x /docker-entrypoint.sh

# Container-side listeners. Remap on the host with -p HOST:CONTAINER and announce
# the host ports to clients with PUBLIC_BASE_URL / MQTT_*_PUBLIC_PORT.
ENV HTTP_ADDR=:8080 \
    METRICS_ADDR=:9102 \
    MQTT_ADDR=:1883 \
    MQTT_WS_ADDR=:8083 \
    MQTT_TLS_ADDR=:8883 \
    SQLITE_PATH=/opt/iot-platform/lib/platform.db
EXPOSE 8080 9102 1883 8083 8883

# lib: SQLite database + firmware; etc: config.yml written by the setup wizard + certs.
VOLUME ["/opt/iot-platform/lib", "/opt/iot-platform/etc"]

HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 \
  CMD wget -qO- "http://127.0.0.1:${HTTP_ADDR##*:}/healthz" >/dev/null || exit 1

ENTRYPOINT ["/docker-entrypoint.sh"]
CMD ["/opt/iot-platform/bin/platform"]
