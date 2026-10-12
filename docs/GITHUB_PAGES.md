# Static Zero-Knowledge edition (GitHub Pages)

The repository includes a **separate, browser-only WASM edition** of insomniaXXX.
It uses the existing Go/WASM encryption, manifest, RLNC encoding/decoding and
locally signed Blossom BUD-02 uploads. All file bytes are encrypted/decrypted
inside the browser. No insomniaXXX Go server is required after the static
website has been built.

## Enable GitHub Pages

1. Open **Settings → Pages** for this repository.
2. Choose **GitHub Actions** as the build/deployment source.
3. On a successful `main` CI run, the `.github/workflows/pages.yml` workflow
   builds the Go WASM bundle, exports the static site and deploys it.
   You can also run the **pages** workflow manually from the Actions tab.
4. The usual project-site URL is
   `https://ihatemyfcklife.github.io/insomniaxxx/`
   after the first successful Pages deployment.

The default configured HTTPS storage origins are
`https://cdn.hzrd149.com`, `https://nostr.download` and
`https://blossom.poster.place`. You may override them by creating the GitHub
Actions **repository variable** `PAGES_BLOSSOM_RELAYS` containing a
comma-separated list of **2 or 3 trusted independent HTTPS origins**.
The exporter validates that configuration and fails the build if it is unsafe
or invalid. These are public destinations, **not private keys**.

### Local build

With the matching Go toolchain installed, from the repository root:

```sh
cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" internal/app/static/wasm_exec.js
GOOS=js GOARCH=wasm go build -trimpath -ldflags='-s -w' -o internal/app/static/app.wasm ./cmd/wasm
go run ./cmd/pages -out dist/pages
node tests/pages-static.test.cjs
```

Serve `dist/pages` from a local HTTP server (including from a nested path),
not directly using `file://`. The output contains `index.html`,
`view.html`, `relays.html`, `.nojekyll` and a `static/` directory
with the WASM runtime, bundle, CSS, scripts and artwork.

### Static share links

A browser-only upload produces:

```text
https://ihatemyfcklife.github.io/insomniaxxx/view.html?hash=<64_hex_ciphertext_hash>#<43_character_secret>
```

The encrypted ciphertext and RLNC manifest/shards remain on Blossom. The
fragment secret is kept locally by the browser and never added to Blossom
requests. The built-in static viewer reconstructs and decrypts the file in
WASM using the same file format as the Go-hosted Zero-Knowledge edition.

**Do not** replace this URL with a `/d/<hash>#<key>` route on GitHub Pages:
GitHub Pages does not run the Go router or provide server rewrites. Existing
server-generated links can be opened on the Go-hosted site; their hash/key can
also be used to form a static `view.html?hash=…#…` link.

### Boundaries and trade-offs

- JavaScript and WebAssembly must be enabled. There is no Tor/No-JS server-side
  encryption, OnionGuard, CAPTCHA, upload proxy or app-side rate limiting.
- Uploads use direct HTTPS CORS requests to trusted configured Blossom relays.
  The relays can see the browser's public IP (or Tor exit) and ciphertext hashes.
  **At least two independent hosts must return valid upload receipts.**
  If CORS, authorization, availability or retention fail, the upload fails
  explicitly; it does not silently use a proxy.
- Downloads also require browser CORS access to at least enough valid shards.
  The app checks ciphertext SHA-256 hashes before decryption.
- A successful GitHub Pages deployment is not proof that external public
  Blossom relays accept uploads, honor retention, or support all browser CORS
  preflights. **Test the complete upload/share/download flow using real HTTPS
  relay hosts**, on desktop and mobile, before relying on it.
- GitHub Pages hosts executable frontend code. Users need to trust the
  repository and the GitHub Pages deployment; WASM alone does not guarantee
  anonymity or prevent compromised frontend code.
- The standard Go-hosted clearnet and native Tor versions stay available;
  this addition does not change their deployment or URL format.

## Verification

The main CI builds the static edition and runs the
`tests/pages-static.test.cjs` integration smoke test. It checks the
generated files, subpath-safe asset URLs, locally signed direct Blossom
transfers, fail-closed CORS behavior, and absence of Go API requests.
`cmd/pages/main_test.go` verifies relay input validation and rendered pages.
