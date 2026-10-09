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

Go 1.24+ is expected by this project.

## Configuration

Copy `.env.example` and configure the deployment environment.

```sh
export BLOSSOM_URLS=https://cdn.hzrd149.com,https://nostr.download,https://blossom.poster.place,https://cdn.sovbit.host
export PUBLIC_BASE_URL=https://files.example
```

Uploads are sent to every configured public Blossom instance. Defaults target general-purpose public Blossom servers that are used for generic nsite/blob storage. Downloads try the instances in order until the encrypted blob is found. Media-only instances such as the free nostr.build endpoint are intentionally not defaults because they reject opaque `application/octet-stream` blobs.

The encrypted file is content-addressed by SHA-256. The share URL contains the blob hash and the random 256-bit share key.

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

The interface is intentionally minimal; the classic HTML forms work without any JavaScript (e.g. Tor Browser Safest), with an optional progressive-enhancement script for local Zero-Knowledge encryption. Pico CSS is bundled with the binary and served locally from `/static/`:

```html
<link rel="stylesheet" href="/static/pico.min.css">
```

OnionGuard's wait-room and CAPTCHA pages use the same Pico CSS link and the same security policy.

## Storage and privacy model

There is no application database, account system, cookie-based file registry, or persistent upload index. The only persistent file storage is the configured public Blossom instances.

The server encrypts the complete file before sending it to Blossom. Blossom receives opaque encrypted bytes and the application uses their SHA-256 as the blob identifier.

Because the interface is strictly no-JS, the share key is in the URL path instead of a URL fragment. This means the application server sees the key during a download request; this design is not zero-knowledge transport.

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
<!-- release smoke test -->
