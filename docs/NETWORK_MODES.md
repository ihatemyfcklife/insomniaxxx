# Clearnet local processing and onion server processing

| Entry point | Encryption, RLNC, decryption | File transport | Share key |
| --- | --- | --- | --- |
| Clearnet, default | Browser WASM | Browser to configured HTTPS Blossom relays | Browser fragment |
| Clearnet, proxy selected | Browser WASM | Encrypted bytes through insomniaXXX | Browser fragment |
| Onion service | Server | Server to Blossom relays | Visible to the server |

Clearnet relay authorization signatures are also generated locally in WASM.
The app still serves the interface/WASM and the relay list and may apply
OnionGuard admission. This is local file processing, not an offline application:
remote relay storage, CORS and the initial app download remain necessary.

Direct mode requires two verified independent-host upload receipts. It tries
the configured spare relay on failures. If direct transfer cannot succeed,
the user must select the encrypted proxy and retry; it never silently routes
file bytes through the app. The preference is stored per tab where available.
Direct relays see the client's network address and ciphertext hashes. Proxy
mode changes transport only and never sends decryption keys.

## Deploy two instances

Use a separate instance for each origin with an explicit `PUBLIC_BASE_URL`:

- Clearnet: `APP_ENV=production`, `PUBLIC_BASE_URL=https://files.example`,
  `ONIONGUARD_COOKIE_SECURE=true`.
- Tor: `APP_ENV=production`, `PUBLIC_BASE_URL=http://YOUR_V3_ADDRESS.onion`,
  `ONIONGUARD_COOKIE_SECURE=false` for native onion HTTP. Expose this instance
  through the Tor hidden service only, not through the public clearnet ingress.

The configured origin decides processing mode. An arbitrary Host header cannot
override it. With no configured origin, development uses the request host.
Tor Browser visiting a clearnet URL still uses clearnet mode: reliable browser
or Tor-exit detection is not available and is not attempted.

No-JS onion uploads and ordinary onion `/d/hash/key` downloads work without
WASM. A clearnet `#key` link opened on the onion service requires explicit
submission of its key to enable server decryption. A small optional script
prefills that field but never submits it; with JavaScript disabled, paste the
part after `#` manually. The user is told that the server sees plaintext/keys.

Clearnet blocks plaintext upload and decryption POST routes. Old `/d/hash/key`
links redirect to `/d/hash#key` for local decryption; the original legacy URL
has already disclosed its key to the server/ingress, so this redirect does not
retroactively make it secret. Raw/download legacy query flags become the local
viewer, from which the user can download the decrypted file.

## Capacity and privacy

Clearnet direct mode removes application file bandwidth and cryptographic CPU
from the normal transfer path. It does not prove unlimited traffic: browser
RAM/CPU, relay capacity/CORS, app admission and proxy usage still matter.
Onion processing needs bounded concurrency, temporary storage, RAM, CPU and
relay bandwidth; retain the production readiness and sustained-load gates.
Redact `/d/hash/key` at every onion ingress, access log and trace collector.

CI checks native Go regressions, frontend direct/proxy routing and failures,
and invokes the actual built WASM authorization export using Node. Real
Tor Browser, Tor networking and relay CORS remain deployment smoke tests.
