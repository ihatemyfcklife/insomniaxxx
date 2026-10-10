FROM golang:1.26-alpine AS build

WORKDIR /src
ENV GOTOOLCHAIN=local

# Resolve exactly the module versions recorded in go.mod/go.sum. Do not
# silently upgrade dependencies to @main or @latest during image builds.
COPY go.mod go.sum ./
RUN go mod download && go mod verify

COPY . .

ARG VERSION=dev

# The stylesheet is checked into the repository at a fixed Pico CSS version.
# Generate the Wasm binary with the runtime bridge shipped by this Go toolchain.
RUN cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" internal/app/static/wasm_exec.js && \
    GOOS=js GOARCH=wasm go build -trimpath -ldflags='-s -w' -o internal/app/static/app.wasm ./cmd/wasm

# Static server binary; VERSION can be set to a release tag or commit SHA.
RUN mkdir -p /out && \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w -X anonblossom/internal/web.Version=${VERSION}" -o /out/anonblossom ./cmd/anonblossom

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build --chown=nonroot:nonroot /out/anonblossom /anonblossom
EXPOSE 8080
ENV LISTEN_ADDR=:8080
USER nonroot:nonroot
ENTRYPOINT ["/anonblossom"]
