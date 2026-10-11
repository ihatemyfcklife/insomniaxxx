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

Go 1.26.0 or newer is required (see the `go` directive in `go.mod`). The Docker build uses `golang:1.27.2-alpine`, and CI tests with Go 1.27.x to match the production build toolchain.

## Configuration

Copy `.env.example` and configure the deployment environment. For public deployments, follow the [production checklist](#production-deployment-checklist).

```sh
export BLOSSOM_URLS=https://cdn.hzrd149.com,https://nostr.download,https://blossom.poster.place,https://cdn.sovbit.host
export PUBLIC_BASE_URL=https://files.example
export ONIONGUARD_COOKIE_SECURE=true
```

New shards are placed on two independent Blossom hosts, rotating their selected pair across up to three configured hosts; the small encrypted manifest is replicated separately. Defaults target general-purpose public Blossom servers used for generic nsite/blob storage. Downloads locate ciphertext shards across the configured instances and reconstruct the file from linearly independent shards. Media-only instances such as the free nostr.build endpoint are intentionally not defaults because they reject opaque `application/octet-stream` blobs.

Every new upload, **regardless of size (including zero bytes)**, is encrypted and split into at least six RLNC shards (K=4 systematic + 2 parity), distributed across at least two independent Blossom relay hosts. With three healthy relays, shard pairs rotate (A+B, B+C, C+A); no relay must store a complete encrypted file. The short encrypted manifest is replicated so recipients can reconstruct from K linearly independent shards. Each shard has two distinct-host receipts. Old monolithic share links remain readable.

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

The classic HTML form posts the file to the server, which encrypts it with `badcrypt` (ChaCha20-Poly1305 AEAD) and **always** splits the ciphertext into RLNC shards before distributing at least two copies of each shard to different configured Blossom hosts. The share link carries the key in the URL path: `/d/<hash>/<key>`.

No JavaScript runs at any point, so this path works in Tor Browser at Safest level and in any browser with scripting disabled. It is also the fallback path: if the Wasm bundle is missing or fails to load, the page silently falls back to the plain HTML form. Because the upload, the preview, and the download are all handled by the server in this mode, the server sees the plaintext file, its name and MIME type, and the share key.

### Zero-Knowledge mode (browser-side, progressive enhancement)

When WebAssembly is available, `boot.js` loads `app.wasm` and takes over the form: key generation, AEAD encryption, and RLNC encoding all run locally in the browser for **all file sizes**. Only already-encrypted bytes leave the browser. The app first attempts direct browser-to-Blossom transfers using short-lived hash-scoped Nostr authorizations, and falls back to `POST /upload-raw` when a relay does not support browser CORS uploads. The share link keeps the key in the URL fragment: `/d/<hash>#<key>`. On download, the browser retrieves ciphertext directly from configured relays if the recipient opts into direct relay access, otherwise it uses the app proxy (`?cipher=1`), then decrypts and decodes locally.

Browsers never send URL fragments to a server, so in this mode the server relays ciphertext only and never receives the plaintext or the share key. This mode requires JavaScript and WebAssembly: it is unavailable in Tor Browser at Safest level, and its guarantees apply only to files that were actually uploaded through this path (files uploaded in no-JS mode were encrypted by the server).

## Cryptographic format and compatibility

The file and manifest use ChaCha20-Poly1305, **not XChaCha20-Poly1305**. Manifest format v2 derives a separate encryption key from the share key, eliminating the previous key-and-nonce reuse between RLNC file and manifest. Legacy v1 manifest links remain readable for compatibility, but their existing ciphertext is not retroactively made safe. Re-upload sensitive legacy files and distribute new links.

Zero-Knowledge mode generates QR codes locally in WebAssembly. Share links have bearer secrets in URL fragments; never send complete share links to server-side QR endpoints, access logs, analytics, or error reporting. No-JS mode intentionally exposes the key to the server and must be deployed with strict URL log redaction.

## RLNC availability and limits

Every new share is distributed as RLNC shards even if the original plaintext is tiny. This costs at least six shard transfers and two independent receipts per shard, so small-file uploads have higher overhead than before.

The erasure coding uses binary XOR masks over **GF(2)**, not Reed-Solomon or GF(2^8). It is **not MDS**: for the classic `k=4, n=6` profile, 3 of 15 possible pairs of missing shards cannot be recovered from only four remaining shards. In general, recovery requires at least `k` **linearly independent** shards, not any arbitrary `k` shards. Availability is also subject to Blossom retention and relay uptime; the service cannot promise a durability SLA.

The frontend reads the exact server upload ceiling and rejects files above it before buffering or encrypting. The anonymous `/upload-raw` endpoint limits each encrypted shard separately and is not a per-user storage quota. OnionGuard, network rate limiting and proxy resource limits are required to mitigate abuse.

When the **No-JS Mode** button is selected, the form submits natively to the server. The Zero-Knowledge button enables local WebAssembly encryption.

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
- **Zero-Knowledge mode limits the server to ciphertext.** With successful direct transfers the encrypted file bytes go from browser to Blossom without passing through insomniaXXX; otherwise the server proxies encrypted bytes and can observe their size, hashes, and request timing. In neither case does it receive the plaintext or the share key, which stays in the browser's URL fragment. During OnionGuard CAPTCHA navigation it may temporarily be held in per-tab sessionStorage, then removed when the link is restored. This protects file content and the key from the server — not from anything else.
- **Neither mode hides metadata.** Your IP address (or Tor circuit), request timing, ciphertext size, and blob hash are visible to the application server and to every Blossom instance that serves the blob. This project makes no anonymity or traffic-analysis guarantee; use Tor or a VPN if that is part of your threat model.
- **Share links are bearer secrets.** Anyone holding the complete link — key included — can decrypt the file. There are no accounts, hence no access list and no revocation: a link keeps working until the blob disappears from every configured relay, and copies already made by readers cannot be recalled.
- **Promised:** stored bytes are encrypted with ChaCha20-Poly1305 before they reach Blossom in both modes, and no server-side database of files, keys, or users exists. **Not promised:** that the server operator cannot read files uploaded in no-JS mode, that metadata is hidden, or that any particular deployment's operator is trustworthy — review the code or run your own instance.

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

Share pages embed their PNG QR code directly as a `data:` image so the bearer key is never sent to `/qr?data=...`. A separate `/qr` endpoint remains available for non-secret input (no inline SVG), so it still displays in Tor Browser's Safest mode where `image.svg.enabled` is false — without JavaScript or a third-party QR service.

## Public server compatibility

The encrypted object is intentionally uploaded as `application/octet-stream`: it is an opaque Blossom blob, not a JPEG, audio file, or video. Public media-only services may reject it even when they implement Blossom. Configure `BLOSSOM_URLS` with servers that accept generic Blossom blobs. Current defaults include `cdn.hzrd149.com`, `nostr.download`, `blossom.poster.place`, and `cdn.sovbit.host`; those are used by current Nostr/Blossom tooling for generic site/blob storage.

When upgrading from an older release, update an existing `.env` too: values already present in `.env` override the new defaults. Replace any old media-only list such as `blossom.nostr.build,blossom.primal.net` with the generic Blossom defaults shown above.


## Streaming encryption and bounded RLNC upload workers (WASM)

WASM uploads prefer a streaming-compatible cipher encoder: `File.slice()`
reads **at most 1 MiB of plaintext per browser operation**, while the Go
WebAssembly encoder emits individually authenticated packets compatible with
the existing ABLOBSH1 decryptor. SHA-256 is accumulated incrementally. The
ciphertext is stored as a browser `Blob` rather than buffered as an extra
full-sized Go `bytes.Buffer`; direct and proxied uploads can send the Blob
without first copying it all into one JavaScript array. The fallback path
remains for a cached/older WASM bundle.

**Important memory limit:** For files of every size the existing RLNC encoder
still needs the **entire ciphertext in memory** to build the systematic
and parity shards. This upgrade removes the initial full-size *plaintext*
buffer in the browser, but **does not yet offer constant-memory RLNC encoding
or streaming decryption**, and the browser can still exhaust RAM on large
files/older mobile devices.

RLNC shard uploads now use **three concurrent workers** (never more than
four), return hashes in original manifest order regardless of which shard
finishes first, and refuse to publish a share link when any shard upload
fails. Every shard and final encrypted manifest still requires two valid
independent relay receipts (or the existing server proxy fallback).

The browser transport remembers recent CORS/relay failures during the page
session and prioritizes healthy configured HTTPS relays; it does **not**
probe unknown relays or transmit share keys. Actual sustained throughput and
large-browser-memory improvements require staging measurements.

## Browser-to-Blossom transport (WASM)

With JavaScript/WebAssembly enabled, insomniaXXX first attempts to send and
retrieve **ciphertext only** directly between the browser and up to three
explicitly configured HTTPS Blossom relays. This removes the main application
backend from the file-byte path **when those relays permit CORS**.

- The browser encrypts and computes SHA-256 locally. `POST /direct-upload-auth`
  returns per-relay, single-hash **short-lived Nostr BUD-02 authorizations**.
  No ciphertext bytes, plaintext or share keys are sent to this endpoint.
- The browser PUTs encrypted bytes to the relays using its own network
  connection. At least **two distinct hosts must return a matching SHA-256
  receipt** before a share link is produced; otherwise the browser retries
  via the existing same-origin `/upload-raw` proxy.
- Browser downloads try `GET https://<configured-relay>/<hash>` with
  `credentials: omit`, disabled redirects and `referrerPolicy: no-referrer`.
  The full downloaded ciphertext is checked against the expected SHA-256 in
  WASM **before decryption**. Corrupted/non-CORS/slow relay responses fall
  back to `/d/<hash>?cipher=1`, where the server verifies storage too.
- New uploads use RLNC shards and an encrypted manifest at every size. Legacy
  monolithic blobs are still downloadable. Server-side No-JS mode encrypts
  locally on the server and also distributes every new file as RLNC shards.
- **Privacy trade-off:** direct requests expose a client's IP or Tor exit
  address and encrypted blob hash to the relay. Users can tick **Private relay
  access** to proxy via insomniaXXX instead; this preference is stored only
  in same-tab `sessionStorage`. **New recipients opening a share link
  default to proxy mode**, so a direct request cannot disclose an address
  before consent; they can uncheck to opt in to direct downloads. Uploaders
  see the disclosure before submitting a WASM upload (direct by default).
  The browser-to-relay link never contains the share/decryption key. Users
  needing the strongest anonymity should choose the proxy, or use Tor and
  trusted relays.
- Relay support is not guaranteed: **CORS must allow the site origin, PUT/GET,
  `Authorization`, `X-SHA-256`, and `Content-Type`**; relays must accept
  hash-scoped ephemeral BUD-02 upload authorizations and expose upload JSON
  receipts. Ordinary public relays may refuse some or all of these requests,
  in which case files still transfer through insomniaXXX.
- `/direct-relays` deliberately returns configured seeds only, never arbitrary
  third-party URLs embedded in a manifest or discovered via Nostr. Use at
  least **two distinct trusted HTTPS relay hosts** in production.
- Transport authorization goes through OnionGuard. Rate-limit that endpoint
  at the reverse proxy; do not log its Authorization-bearing response.
  Direct relay traffic bypasses your reverse-proxy rate limits and relies on
  the storage relays' own restrictions.

This feature reduces backend transfer saturation **only for successful direct
traffic**. It is not a guarantee of 1,000 simultaneously working transfers:
browser memory, relay quotas and bandwidth, CORS compatibility and load
testing remain required.

## Scaling toward 1,000 simultaneous file transfers

The server now applies **independent admission limits** to:
- encrypted WASM/ciphertext downloads (`DOWNLOAD_CONCURRENCY`, default 64);
- server-side No-JS decryption and previews (`HEAVY_DOWNLOAD_CONCURRENCY`, default 4);
- uploads (`UPLOAD_CONCURRENCY`, default 8);
- costly RLNC upload encoders (`RLNC_UPLOAD_CONCURRENCY`, default 1).

Encrypted downloads are SHA-256 verified into bounded temporary files **before**
being served to browsers; they do not allocate a complete encrypted blob in
the process heap. `SPOOL_BUDGET_BYTES` (default 1 GiB) constrains the effective
simultaneous ciphertext streams to the floor of the budget divided by the
configured maximum encrypted blob size. New requests receive HTTP 503 with a
`Retry-After` header when admission is exhausted.

Run multiple application replicas behind a trusted HTTPS load balancer. Set
`ONIONGUARD_REDIS_ADDR` and (if needed) `ONIONGUARD_REDIS_PASSWORD` on
**every** replica to share admission/session state; keep Redis private and
protected. Without Redis, OnionGuard memory sessions are instance-local and
load balancing is not guaranteed to preserve CAPTCHA or visitor sessions.
No upload or download data should be stored in Redis.

Plan tmpfs capacity for the spool budget **plus** active multipart uploads,
RLNC intermediate files and decrypted No-JS cache. The 1 GiB tmpfs example
below is a development baseline, **not** adequate for 1 GiB spool + overhead;
size the filesystem and container disk quota using measured load peaks.
Use privately operated, independently managed Blossom staging relays. Public
relays can rate-limit or reject uploads; application performance alone cannot
guarantee service-wide throughput.

An included `tests/load/cipher-download.mjs` k6 scenario generates 1,000
virtual download clients; see [load testing instructions](tests/load/README.md).
A virtual-user count is not proof of 1,000 active transfers. A production claim
requires a real mixed upload/download test across distinct file sizes and both
encryption modes, plus failover, memory, disk, ingress, OnionGuard and relay
verification. The CI unit tests do **not** certify 1,000 simultaneous file
transfers on any given deployment.

## Production deployment

The following requirements consolidate the former deployment checklist. **A successful GitHub release or CI check is not proof of production readiness.** Verify the real deployment before serving public traffic. For branch protection and release operations, see [Production release controls](.github/PRODUCTION_RELEASE.md).

### Release and CI safeguards

GitHub releases are published only after a successful GitHub Actions CI check for the exact `main` commit. The workflow verifies build artifacts and SHA-256 checksums before creating the tag. Manual releases are restricted to `main` and must satisfy the same commit check.

Configure `BLOSSOM_URLS` with **at least two genuinely independent, trusted relay operators**. Direct files and RLNC shards must be replicated to distinct relays before a share link is shown; a successful receipt does not guarantee permanent availability.

### Production deployment checklist

This application has no file database to back up, but it does process sensitive file data and temporarily writes plaintext to local temporary files in server-side (no-JS) mode. Treat deployment configuration and logs as part of the security boundary.

#### Required configuration

- Set `PUBLIC_BASE_URL` to the exact public HTTPS origin used by clients.
- Set `ONIONGUARD_COOKIE_SECURE=true` when served over HTTPS. Keep it false only for a native HTTP .onion service.
- Keep `BLOSSOM_URLS` limited to trusted, publicly reachable Blossom servers that accept generic `application/octet-stream` blobs. The application rejects private/special-use relay destinations and does not follow redirects.
- `MAX_UPLOAD_BYTES` must be positive and at most 67,108,864 bytes (64 MiB). `UPLOAD_CONCURRENCY` must be between 1 and 32. Encrypted downloads and No-JS decryption have separate, resource-budgeted concurrency controls; see [scaling](#scaling-toward-1000-simultaneous-file-transfers).

#### Required redundancy, privacy, and edge controls

- Configure **at least two independent and approved** `BLOSSOM_URLS` origins. The server requires two successful receipts for direct blobs and two distinct storage copies per RLNC shard. If either upload target rejects a shard, no working link is issued. Published ciphertext cannot be revoked.
- Keep trusted relay allowlists and TLS configuration audited. Client-supplied RLNC manifest relay URLs are never allowed to turn the service into a general HTTP proxy; destinations must be known configured or discovered relay origins and pass SSRF checks.
- The decrypted plaintext cache is bounded by **two completed entries / 128 MiB** in addition to its three-minute TTL; the reverse proxy and container still need disk/CPU/memory limits and load testing.
- Zero-Knowledge CAPTCHA pages use a static local script to retain a pending URL-fragment key in **per-tab sessionStorage for at most five minutes**. It is cleared when the successful view opens. Confirm with a new browser session that no request or monitoring log carries the key.
- Set per-client and aggregate rate limits at the edge for `/upload`, `/upload-raw` and `/d/`; the per-blob size limit does not provide per-account quotas on this anonymous service.

#### Production mode and deployment gate

Set `APP_ENV=production`. This enforces `PUBLIC_BASE_URL` and rejects non-HTTPS public origins (native HTTP `.onion` addresses are allowed). **Do not omit this variable**; it is intentionally optional for local development. Set a concrete `PUBLIC_BASE_URL`, not a value derived from client-controlled `Host` headers.

In the deployment example below, pass `--env APP_ENV=production` and use an `.env` containing a canonical `PUBLIC_BASE_URL` and matching `ONIONGUARD_COOKIE_SECURE=true`.

The repository CI covers unit/race tests, syntax tests, dependency scans and builds. To approve public traffic, also verify an *actual* browser over the deployment's TLS/Tor ingress: upload and recover a small and maximum-size file in both modes, check that Zero-Knowledge keys do not appear in network requests, ensure relay failures expire cleanly, confirm no request path or referrer containing a secret is logged, and measure peak RSS and temporary disk consumption under simultaneous traffic. These tests cannot be inferred from green CI.

#### Network and reverse proxy

Terminate public TLS at a maintained reverse proxy or ingress. Forward requests to the application over loopback or a private container network, and do not expose the application port directly to the internet. Configure the proxy with upload/body limits consistent with `MAX_UPLOAD_BYTES`, connection/rate limits, and timeouts suitable for the expected file size and network (including Tor).

**Do not log full request targets or referrers.** In no-JS mode, a bearer share key is part of the URL path (`/d/<hash>/<key>`); ordinary access logs can therefore leak the key. Log a route template, HTTP method, status, and duration instead. Never log upload contents, share keys, decrypted filenames, or full share links. Ensure error monitoring does not capture request URLs.

Set `PUBLIC_BASE_URL` to the canonical origin and test redirects/cookies through the real proxy. The application does not itself terminate HTTPS.

#### Container and host

The Docker image runs as the distroless non-root user. Run it with a read-only root filesystem and an ephemeral, non-executable temporary filesystem large enough for uploads and short-lived decrypted cache files. Do not mount the temporary directory persistently or back it up. Use explicit memory, CPU, PID, and disk limits; the RLNC upload path keeps intermediate buffers in memory, so provision enough memory for the configured upload size and concurrency. When increasing concurrent transfers, provision memory and `/tmp` based on measured worst-case work: verified ciphertext spooling, multipart uploads, RLNC buffers, and No-JS plaintext caching. A 1 GiB `/tmp` is insufficient for the default 1 GiB spool budget *plus* other work.

Example for a locally built image (adapt host limits and the proxy to your platform):

```sh
docker build --build-arg VERSION="$(git rev-parse --short HEAD)" -t anonblossom:release .
docker run --detach --name anonblossom \
  --read-only --tmpfs /tmp:rw,noexec,nosuid,size=4g \
  --memory=4g --cpus=2 --pids-limit=128 \
  --restart unless-stopped --env APP_ENV=production --env-file .env \
  -p 127.0.0.1:8080:8080 anonblossom:release
```

Do not put secrets directly in image layers or Docker build arguments. Keep environment files readable only by the deployment identity.

#### Privacy and service guarantees

- **No-JS mode is not zero-knowledge:** the server sees plaintext and the share key during upload/download, and decrypted data can exist briefly in `/tmp`.
- **WASM mode** encrypts/decrypts client-side only when the browser actually loads and runs the WASM flow. Test that mode explicitly; do not infer it from a successful homepage response.
- The application has no database or upload index. Public Blossom operators control the persistence and availability of stored ciphertext. There is no application-level revocation or backup for objects stored on those external relays.
- The application and relays see network and blob metadata. Do not promise anonymity or resistance to traffic analysis.

#### Upload receipts and relay limitations

Only relays returning a well-formed Blossom descriptor with a matching SHA-256 are treated as a successful upload. An accepted receipt does not guarantee permanent storage; check actual retrieval and retention against the relays you operate or trust. Outbound relay operations have overall deadlines, and media is only previewed inline for explicitly allowlisted passive MIME types. Treat `/upload-raw` as an anonymous encrypted-blob endpoint: configure reverse-proxy rate and body limits, and do not mistake the frontend file limit for a cryptographically enforced per-user storage quota.

#### Security migration after the manifest v2 fix

Manifest v1 and file encryption previously used the same share key with a reset ChaCha20-Poly1305 nonce counter. Version 2 manifests now use a domain-separated key. Older v1 manifest links remain decryptable for compatibility, **not** cryptographically repaired. Treat previously shared sensitive RLNC files as potentially exposed: re-upload those files after deploying the fix and redistribute their new links. Rotating the service binary alone cannot fix existing ciphertext stored on external Blossom relays.

The Zero-Knowledge QR code is generated inside the browser. Verify with browser developer tools that no request to `/qr?data=` carries a fragment-bearing share link. For all deployments, redact request paths, query strings and referrers from every proxy, CDN, WAF, and monitoring service.

#### Release gate

Before production traffic, all of the following should be evidenced for the exact release commit:

1. CI passes formatting, `go test -race ./...`, `go vet ./...`, `govulncheck`, the Linux server build, the WASM build, and the Docker build.
2. Run a staging smoke test through the real TLS proxy for upload, preview, download, expiry/cache cleanup, the WASM zero-knowledge flow, and the no-JS flow.
3. Test with the maximum allowed upload size, simultaneous uploads/downloads, relay timeouts/failures, malformed and oversized relay responses, and graceful SIGTERM shutdown.
4. Confirm HTTPS and cookie behavior; verify proxy/application logs redact bearer keys and referrers; verify disk/memory/CPU limits, alerts, and recovery/redeployment procedures.
5. Review any vulnerability scan findings and obtain an independent security review appropriate to the threat model before advertising this service as secure or anonymous.

CI and code-level tests do not prove that a particular deployment is safe. A release is not production-approved until the staging checks, resource sizing, logging controls, and actual HTTPS deployment have been verified.

## License

Copyright 2026 ihatemyfcklife.

Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with the License. You may obtain a copy of the License in the repository at [LICENSE](LICENSE).

Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the specific language governing permissions and limitations under the License.
<!-- release smoke test -->
