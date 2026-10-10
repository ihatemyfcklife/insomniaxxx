# Staging load test: 1,000 encrypted file transfers

This is a reproducible **external** load-test entrypoint, not an assertion that
a single application instance can sustain 1,000 concurrent transfers.

## Setup

1. Build and deploy the same release SHA across a staging cluster. Use managed,
   isolated Blossom relays with adequate capacity, not the public defaults.
2. Configure a private, HA Redis service using ONIONGUARD_REDIS_ADDR and
   ONIONGUARD_REDIS_PASSWORD on every replica. Verify session handoff across
   replicas and configure strict network access to Redis.
3. Populate a synthetic encrypted blob on at least two staging Blossom relays.
   Record its 64-character SHA-256 hash (NOT a share key).
4. Establish valid staging-only OnionGuard admission sessions. Cookies are
   security-sensitive; supply them securely and never upload the results or
   exported environment to a public dashboard.
5. Allocate enough temporary disk and outbound bandwidth to the app instances,
   plus enough CPU/RAM/network on multiple independent load generators.

## Run

Install [k6](https://grafana.com/docs/k6/latest/) on a dedicated load generator.

```sh
K6_BASE_URL=https://staging.example.com \
K6_BLOB_HASH=<64-hex-digit-seeded-ciphertext-sha256> \
K6_SESSION_COOKIE=<valid-staging-only-cookie> \
K6_VUS=1000 K6_DURATION=5m \
k6 run tests/load/cipher-download.mjs
```

**Important:** k6 constant VUs are not necessarily 1,000 *simultaneously
active transfers* if requests finish quickly or OnionGuard challenges block
requests. To validate 1,000 active in-flight transfers, use enough file size,
network shaping, per-replica active-transfer metrics, and several load
generators. Verify all traffic returns real ciphertext, not a CAPTCHA page.
For full claims, separately run uploads (No-JS and ZK), plaintext downloads,
64 MiB RLNC, 1-relay-failure, 1-node-failure, slow clients, Tor ingress,
and 60-minute sustained mixed traffic.

A 1,000-goroutine Go admission unit test is included in the repository; it
only verifies bounded concurrency/refusal handling, not throughput. No test
may write thousands of blobs to public relays without operator permission.

## Mixed 1,000-VU scenario

Prepare TWO staging-only fixtures:
- `K6_CIPHER_FILE`: a correctly encrypted cryptbox blob or RLNC shard accepted
  by `/upload-raw`. Do not pass plaintext to this endpoint.
- `K6_NOJS_FILE`: a synthetic (non-sensitive) plaintext file accepted by
  `/upload`.
- `K6_NOJS_SHARE_PATH`: an existing No-JS /d/hash/key path on staging.
  **Never** share this link or record the full request URL in logs.
- `K6_BLOB_HASH`: the 64-hex-digit hash of an existing seeded encrypted blob.

```sh
K6_BASE_URL=https://staging.example.com \
K6_BLOB_HASH=<staging-ciphertext-sha256> \
K6_NOJS_SHARE_PATH=/d/<staging-hash>/<staging-test-key> \
K6_CIPHER_FILE=tests/fixtures/synthetic-encrypted.blob \
K6_NOJS_FILE=tests/fixtures/synthetic-plain.bin \
K6_SESSION_COOKIE=<valid-staging-only-cookie> \
K6_VUS=1000 K6_DURATION=5m \
k6 run tests/load/mixed-transfers.mjs
```

This splits virtual users equally across ciphertext downloads, No-JS plaintext
downloads, encrypted uploads and No-JS plaintext uploads. It exercises HTTP
routes but does **not** itself prove browser WASM/CAPTCHA semantics. Do not
run until a secured staging cluster, Redis session strategy, sufficient test
generators, and PRIVATE Blossom storage are ready. Do not store test keys in
shell history or log complete request URLs.

## Capacity notes

- DOWNLOAD_CONCURRENCY is the configured maximum verified ciphertext streams
  per replica; the runtime additionally caps streams by the verified-disk
  SPOOL_BUDGET_BYTES and maximum encrypted blob size.
- HEAVY_DOWNLOAD_CONCURRENCY separately bounds plaintext decryptions.
- Uploads and RLNC encoders have separate concurrency limits, CPU and memory.
- Per replica, temporary storage must exceed the configured spool budget PLUS
  HTTP multipart, active uploads and no-JS cache; start by measuring peak use.
- OnionGuard rate limits and WAF policies can throttle test traffic even when
  the HTTP server itself has spare capacity.
- Bandwidth, relay retention and TLS/cookie/privacy correctness remain external
  production release gates.

Never label a deployment "1,000 transfers ready" without an actual mixed-mode
run with near-zero errors, checksum validation, measured p95/p99 latency,
observability, failure recovery and staging security verification.
