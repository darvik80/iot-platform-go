.PHONY: web build run

web:
	cd web && npm ci && npm run build

build: web
	go build -o bin/platform.exe ./cmd/platform

run: build
	./bin/platform.exe
