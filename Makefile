.PHONY: web build build-linux run

web:
	cd web && npm ci && npm run build

build: web
	go build -o bin/platform.exe ./cmd/platform

# Static binary for Cogitor VPS systemd (iot.cogitor.dev)
build-linux: web
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o bin/platform ./cmd/platform

run: build
	./bin/platform.exe
