# insomniaXXX

Decentralized anonymous file sharing in Go.

## Dependencies

The runtime uses:

- `github.com/ihatemyfcklife/badcrypt`
- `github.com/ihatemyfcklife/onionguard`
- `fiatjaf.com/nostr` for Blossom BUD-11 upload authorization

Install them with:

```sh
make deps
```

Go 1.26.0 or newer is required. The `go` directive in `go.mod` is the source of truth: the Docker build is pinned to `golang:1.26-alpine` to match it, and CI builds with the latest stable Go release, which is always at or above that minimum.

## Configuration

Copy `.env.example` and configure the deployment environment.

```sh
export BLOSSOM_URLS=https://cdn.hzrd149.com,https://nostr.download,https://blossom.poster.place,https://cdn.sovbit.host
export PUBLIC_BASE_URL=https://files.example
```

Uploads are sent to every configured public Blossom instance. Defaults target general-purpose public Blossom servers that are used for generic nsite/blob storage. Downloads try the instances in order until the encrypted blob is found. Media-only instances such as the free nostr.build endpoint are intentionally not defaults because they reject opaque `application/octet-stream` blobs.

The encrypted file is content-addressed by SHA-256. The share URL contains the blob hash and the random 256-bit share key — in the URL path in no-JS mode, in the URL fragment in Zero-Knowledge mode (see [Frontend](#frontend)).

## Run

```sh
make run
```

Build:

```sh
make build
```

Test:

```sh
make test
```

Format and static checks:

```sh
make fmt
make vet
```

## Frontend

The interface is intentionally minimal: one page with two possible execution paths. Which one runs depends on whether JavaScript and WebAssembly are available in the browser. Pico CSS is bundled with the binary and served locally from `/static/`:

```html
<link rel="stylesheet" href="/static/pico.min.css">
```

OnionGuard's wait-room and CAPTCHA pages use the same Pico CSS link and the same security policy.

### No-JS mode (server-side)

The classic HTML form posts the file to the server, which encrypts it with `badcrypt` (XChaCha20-Poly1305 AEAD) and, above the RLNC threshold, additionally splits the ciphertext into RLNC shards before storing it on the configured Blossom instances. The share link carries the key in the URL path: `/d/<hash>/<key>`.

No JavaScript runs at any point, so this path works in Tor Browser at Safest level and in any browser with scripting disabled. It is also the fallback path: if the Wasm bundle is missing or fails to load, the page silently falls back to the plain HTML form. Because the upload, the preview, and the download are all handled by the server in this mode, the server sees the plaintext file, its name and MIME type, and the share key.

### Zero-Knowledge mode (browser-side, progressive enhancement)

When WebAssembly is available, `boot.js` loads `app.wasm` and takes over the form: key generation, AEAD encryption, and RLNC encoding all run locally in the browser. Only already-encrypted bytes are uploaded (`POST /upload-raw`), which the server checks at the header level (valid cryptbox header or RLNC shard) and forwards to Blossom. The share link keeps the key in the URL fragment: `/d/<hash>#<key>`. On download, the browser fetches the raw ciphertext (`?cipher=1`) and decrypts and decodes it locally.

Browsers never send URL fragments to a server, so in this mode the server relays ciphertext only and never receives the plaintext or the share key. This mode requires JavaScript and WebAssembly: it is unavailable in Tor Browser at Safest level, and its guarantees apply only to files that were actually uploaded through this path (files uploaded in no-JS mode were encrypted by the server).

## Storage and privacy model

There is no application database, account system, cookie-based file registry, or persistent upload index. The only persistent file storage is the configured public Blossom instances.

In both modes the stored object is ciphertext, identified by the SHA-256 of the encrypted bytes. Blossom relays receive opaque `application/octet-stream` blobs; they never receive file names or plaintext contents.

The two modes differ in who holds the key and who can read the file:

| | No-JS mode | Zero-Knowledge (Wasm) mode |
| --- | --- | --- |
| Encryption happens in | The server | The browser |
| Share key location | URL path: `/d/<hash>/<key>` | URL fragment: `/d/<hash>#<key>` |
| Server sees the plaintext | Yes — on upload and on every preview/download | No |
| Server sees the share key | Yes | No — fragments are not sent to the server |
| Requires JavaScript | No | Yes |

What the design does and does not give you:

- **No-JS mode is not zero-knowledge.** The application server generates the key, encrypts the upload, and decrypts the file again to render previews and stream downloads; decrypted copies sit in an ephemeral on-disk cache for a few minutes. Whoever operates (or compromises) the server can read the file and the key.
- **Zero-Knowledge mode limits the server to ciphertext.** The server still relays the encrypted bytes and can observe their size, hashes, and request timing, but it never receives the plaintext or the key, and the key stays in the browser's URL fragment. This protects file content and the key from the server — not from anything else.
- **Neither mode hides metadata.** Your IP address (or Tor circuit), request timing, ciphertext size, and blob hash are visible to the application server and to every Blossom instance that serves the blob. This project makes no anonymity or traffic-analysis guarantee; use Tor or a VPN if that is part of your threat model.
- **Share links are bearer secrets.** Anyone holding the complete link — key included — can decrypt the file. There are no accounts, hence no access list and no revocation: a link keeps working until the blob disappears from every configured relay, and copies already made by readers cannot be recalled.
- **Promised:** stored bytes are encrypted with XChaCha20-Poly1305 before they reach Blossom in both modes, and no server-side database of files, keys, or users exists. **Not promised:** that the server operator cannot read files uploaded in no-JS mode, that metadata is hidden, or that any particular deployment's operator is trustworthy — review the code or run your own instance.

## Troubleshooting

After cloning the project, install the external modules once with:

```sh
make deps
```

`go test`, `make build`, and `make run` do not modify `go.mod`; dependency installation is kept as an explicit step.

The current `fiatjaf.com/nostr` API generates an ephemeral signing key with `nostr.Generate()`. OnionGuard's standard-library integration creates an `*Engine` with `onionguard.New` and wraps the application with `middleware.Middleware(engine)`.

## UI and upload compatibility

The OnionGuard wait room uses a standard HTML `<meta http-equiv="refresh">`, so the five-second proof-of-patience step advances automatically without JavaScript. OnionGuard documents the same zero-JavaScript mechanism. ([OnionGuard](https://github.com/ihatemyfcklife/onionguard))

The upload authorization follows current Blossom BUD-11 requirements: the `24242` token contains `t=upload`, a five-minute `expiration`, the target `server`, and an `x` tag scoped to the uploaded SHA-256 blob. Blossom currently requires the `x` tag for `PUT /upload`. ([BUD-11](https://github.com/hzrd149/blossom/blob/master/buds/11.md))

Share links include a server-rendered QR code. The QR is a static PNG served by the `/qr?data=...` endpoint (no inline SVG), so it still displays in Tor Browser's Safest mode where `image.svg.enabled` is false — without JavaScript or a third-party QR service.

## Public server compatibility

The encrypted object is intentionally uploaded as `application/octet-stream`: it is an opaque Blossom blob, not a JPEG, audio file, or video. Public media-only services may reject it even when they implement Blossom. Configure `BLOSSOM_URLS` with servers that accept generic Blossom blobs. Current defaults include `cdn.hzrd149.com`, `nostr.download`, `blossom.poster.place`, and `cdn.sovbit.host`; those are used by current Nostr/Blossom tooling for generic site/blob storage.

When upgrading from an older release, update an existing `.env` too: values already present in `.env` override the new defaults. Replace any old media-only list such as `blossom.nostr.build,blossom.primal.net` with the generic Blossom defaults shown above.

## License

Copyright 2026 ihatemyfcklife.

Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with the License. You may obtain a copy of the License in the repository at [LICENSE](LICENSE).

Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the specific language governing permissions and limitations under the License.
<!-- release smoke test -->
