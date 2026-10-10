.PHONY: deps run test race build fmt vet vuln check wasm docker-build

deps:
	go mod download
	go mod verify

run:
	set -a; [ ! -f .env ] || . ./.env; set +a; go run ./cmd/anonblossom

build: wasm
	mkdir -p bin
	VERSION=$$(git describe --tags --abbrev=0 2>/dev/null || git rev-parse --short HEAD 2>/dev/null || echo dev); \
	CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X anonblossom/internal/web.Version=$$VERSION" -o bin/anonblossom ./cmd/anonblossom

test:
	go test ./...

race:
	go test -race -count=1 ./...

fmt:
	gofmt -w ./cmd ./internal

vet:
	go vet ./...

vuln:
	go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 ./...

check: deps test race vet vuln

docker-build:
	docker build --build-arg VERSION=$$(git describe --tags --abbrev=0 2>/dev/null || git rev-parse --short HEAD 2>/dev/null || echo dev) -t anonblossom:local .

wasm:
	test -s internal/app/static/pico.min.css
	cp "$$(go env GOROOT)/lib/wasm/wasm_exec.js" internal/app/static/wasm_exec.js
	GOOS=js GOARCH=wasm go build -trimpath -ldflags='-s -w' -o internal/app/static/app.wasm ./cmd/wasm
