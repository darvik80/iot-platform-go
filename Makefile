.PHONY: web build build-linux run gen-mqtt-tls deb

VERSION  := $(shell cat version.txt 2>/dev/null || echo dev)
COMMIT   := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
LDFLAGS  := -s -w -X main.Version=$(VERSION) -X main.Commit=$(COMMIT)

web:
	cd web && npm ci && npm run build

build: web
	go build -ldflags="$(LDFLAGS)" -o bin/platform.exe ./cmd/platform

# Static binary for Cogitor VPS systemd (iot.cogitor.dev)
build-linux: web
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o bin/platform ./cmd/platform

run: build
	./bin/platform.exe

# Generate self-signed TLS certificates for MQTT broker (requires openssl)
# Usage: make gen-mqtt-tls [DIR=./certs/mqtt] [CN=iot.local]
DIR ?= ./certs/mqtt
CN ?= iot.local
gen-mqtt-tls:
	@bash scripts/gen-mqtt-tls.sh "$(DIR)" "$(CN)"

# Build .deb package (run from WSL or Linux)
deb:
	@bash deb/build.sh
