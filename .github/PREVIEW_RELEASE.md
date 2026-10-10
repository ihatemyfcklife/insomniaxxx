# Preview release policy

insomniaXXX preview builds are distributed as GitHub **pre-releases** rather
than production-approved stable versions. The preview workflow requires
successful `CI / test-and-build` for the exact current main commit, produces
Linux amd64/arm64 and WASM assets and SHA-256 checksums, then publishes with
`prerelease: true` and `make_latest: false`.

The current preview includes all scaling changes merged in PR #16:

- Independent bounded admission of ciphertext downloads, no-JS decryptions,
  ordinary uploads and high-memory RLNC uploads.
- Temporary-disk SHA-256 verification of encrypted downloads prior to serving.
- Concurrent independent Blossom replication and bounded shard workers.
- Optional shared OnionGuard Redis state for horizontally deployed instances.
- Configuration, regression and synthetic k6 load test entrypoints.

**Not yet verified:** 1,000 simultaneous *real file transfers*, mixed-mode
browser behavior under load, staging ingress, distributed relay endurance,
and Tor/HTTPS privacy under peak traffic. Passing CI does not establish any
of these claims; please use controlled beta/staging environments.

The regular `release.yml` workflow still requires an explicit and truthful
staging attestation for skipped production commits. Preview releases do not
modify that production gate.
