Static WebAssembly assets served at /static/.

Run `make wasm` to populate this directory with:
  - wasm_exec.js (official Go Wasm bridge script)
  - app.wasm     (local encrypt/decrypt bundle for cmd/wasm)

This placeholder keeps `//go:embed static/*` valid before the first build.
