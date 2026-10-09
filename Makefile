.PHONY: deps run test build fmt vet check wasm

deps:
	go get github.com/ihatemyfcklife/badcrypt@main
	go get github.com/ihatemyfcklife/badrlnc@main
	go get github.com/ihatemyfcklife/onionguard@main
	go get fiatjaf.com/nostr@latest
	go get github.com/skip2/go-qrcode@latest
	go mod tidy

run:
	set -a; [ ! -f .env ] || . ./.env; set +a; go run ./cmd/anonblossom

build: wasm
	VERSION=$$(git describe --tags --abbrev=0 2>/dev/null || echo dev); go build -ldflags "-s -w -X anonblossom/internal/web.Version=$$VERSION" -o bin/anonblossom ./cmd/anonblossom

test:
	go test ./...

fmt:
	gofmt -w ./cmd ./internal

vet:
	go vet ./...

check: fmt test vet

wasm:
	mkdir -p internal/app/static
	[ -f internal/app/static/pico.min.css ] || curl -sSL -o internal/app/static/pico.min.css https://cdn.jsdelivr.net/npm/@picocss/pico@2/css/pico.fuchsia.min.css
	cp "$$(go env GOROOT)/lib/wasm/wasm_exec.js" internal/app/static/wasm_exec.js
	GOOS=js GOARCH=wasm go build -trimpath -ldflags='-s -w' -o internal/app/static/app.wasm ./cmd/wasm
