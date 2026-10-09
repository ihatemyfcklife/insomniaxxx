FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go get github.com/ihatemyfcklife/badcrypt@main \
    github.com/ihatemyfcklife/badrlnc@main \
    github.com/ihatemyfcklife/onionguard@main \
    fiatjaf.com/nostr@latest \
    && go mod tidy \
    && go mod download
COPY . .

RUN apk add --no-cache curl && \
    curl -sSL -o internal/app/static/pico.min.css https://cdn.jsdelivr.net/npm/@picocss/pico@2/css/pico.fuchsia.min.css

# 1. Copie du bridge officiel et compilation Wasm
RUN cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" internal/app/static/wasm_exec.js 2>/dev/null || \
    cp "$(go env GOROOT)/misc/wasm/wasm_exec.js" internal/app/static/wasm_exec.js && \
    GOOS=js GOARCH=wasm go build -trimpath -ldflags='-s -w' -o internal/app/static/app.wasm ./cmd/wasm

# 2. Compilation du binaire serveur (qui embarque static/*)
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags='-s -w' -o /out/anonblossom ./cmd/anonblossom

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/anonblossom /anonblossom
EXPOSE 8080
ENV LISTEN_ADDR=:8080
ENTRYPOINT ["/anonblossom"]
