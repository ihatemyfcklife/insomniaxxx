# Production readiness review — 2026-10-11

Reviewed baseline: `a4683f545307514130a37b22bdab6656f7124a4b`.
The baseline CI run 38115749831 passed. This is not a capacity measurement.

## Decision

Not yet verified for high-traffic production. No deployment, private staging
relay credentials, hardware limits, or quantified traffic target was available
for this review. Existing production/privacy gates remain in force.

## Concrete fixes

- Upload admission now returns HTTP 503 with Retry-After instead of HTTP 400,
  including the separate large-file RLNC gate.
- Encrypted proxy uploads reuse the multipart ReaderAt/ReadSeeker. They no
  longer copy the complete payload into a second temporary file. The original
  multipart body limits, format checks, SHA-256 and replica receipts remain.
- Concurrent plaintext cache misses no longer consume the completed-entry
  retention quota. Previously four admitted misses could cause the first
  completed decryptions to fail because nothing was yet eligible for eviction.
- Cache hits open files under the eviction lock; a waiter whose entry was
  evicted retries instead of returning a spurious missing-file error.
- Load scripts verify download hashes, encrypted upload receipts, and No-JS
  upload/download roundtrips. HTTP 200 challenge pages no longer pass. Metrics
  omit URL labels; transport/debug logs still require protection.

## Verification

Six Node test files pass locally, including simulated challenge/corruption
responses for the load scripts. Added Go regressions cover four simultaneous
cache misses, retryable admission/recovery, and byte-identical replicated raw
uploads using both RAM-backed and disk-backed multipart files. Go is not
installed in the local audit environment; the PR CI runs these regressions
with the race detector, vet, vulnerability scanning and production builds.
Check the CI of the final PR commit before merging.

## Remaining deployment gates

Follow PRODUCTION_PERFORMANCE_GATE.md and tests/load/README.md on private
staging infrastructure. Record the target requests/second, file-size mix,
active transfers, latency/error objectives and hardware. Exercise sustained
load and relay/node/Redis failures, and measure CPU, RSS, temporary filesystem
peak, bandwidth and generator saturation. The per-path limits still do not
constitute one global disk quota; provision bounded ephemeral storage and
container resource limits. Multi-replica sessions require shared Redis.

Verify real ingress TLS, No-JS path/key redaction, relay CORS/retention and
browser/Tor behavior. These changes neither deploy the service nor certify
1,000 simultaneous transfers. They do not change the production release gate.
