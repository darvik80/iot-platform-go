# syntax=docker/dockerfile:1
FROM node:22-bookworm AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.25-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/internal/webui/dist ./internal/webui/dist
RUN test -f internal/webui/dist/index.html \
 && CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/platform ./cmd/platform

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata \
  && adduser -D -u 1000 platform \
  && mkdir -p /data && chown platform:platform /data
WORKDIR /
COPY --from=build /out/platform /platform
USER platform
EXPOSE 8080
ENV SQLITE_PATH=/data/platform.db
ENV HTTP_ADDR=:8080
ENTRYPOINT ["/platform"]
