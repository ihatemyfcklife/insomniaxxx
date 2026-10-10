# Security audit — insomniaXXX `v0.2.16-rc.1` (2026-10-11)

**Scope:** static code review of the current preview tag, including Go HTTP
handlers, cryptbox integration, Blossom outbound requests, WASM/JS, upload
limits, deployment, CI and release tooling. This is **not** a penetration test,
cryptographic proof, independent third-party review, live-system audit, or
1,000 concurrent transfer certification.

## Findings addressed by security/audit-oct-2026

| Severity | Finding | Mitigation |
|---|---|---|
| HIGH | Docker builder pinned `golang:1.27.0-alpine` while Go 1.27.2 (2026-10-08) contains standard library security fixes. The Go toolchain baked into built static Docker app binaries was behind the patched version. | Pin `golang:1.27.2-alpine`; validate the actual container builder version in CI before accepting an image. |
| MEDIUM | `APP_ENV` values other than literal `production` were accepted silently, so a typo could disable mandatory HTTPS/public origin/relay validation. | Reject unknown application modes, even for local config validation. |
| MEDIUM | Production accepted unencrypted HTTP URLs for configured public Blossom relays, exposing auth events, traffic metadata and payload transfers to network observation/modification, although blobs are encrypted. | Require HTTPS Blossom origins in production; retain HTTP support only in development/test setups. |
| LOW | Static assets and favicon bypassed the shared security headers because `securityHeaders` wrapped only the guarded routes. | Apply security headers to the root router, including statics, without adding OnionGuard to asset paths. |

## Controls examined

- `/upload` and `/upload-raw` have bounded request-body sizes, multipart
  parsing, concurrency admissions and checks on share/blob form.
- Relay destinations are checked for public IPs and rechecked at dial time;
  outbound redirects and proxy redirects are disabled, mitigating SSRF and
  DNS-rebinding attacks.
- Fetch paths bound response bodies and verify ciphertext SHA-256 before
  accepting relay content.
- Server-side plaintext responses use a passive MIME allowlist and attachment
  disposition for other content types.
- The WASM share secret is in the URL fragment. The server-side No-JS
  share key is necessarily in the request URL path.
- The HTTP templates use Go `html/template`; frontend filename display
  uses text nodes or escaping; external HTML/SVG uploads are not rendered
  inline by the No-JS handler.

## Open residual risks — NOT certified as safe

1. **HIGH (deployment-dependent):** The No-JS link includes the decryption
   secret in `/d/<sha256>/<key>`. TLS proxy/CDN/WAF/access logs, trace
   collectors, and browser history can reveal the secret unless paths/referrers
   are redacted at **every** layer. Use `Referrer-Policy: no-referrer`,
   disable query/path logging, and audit the real ingress. The No-JS server
   sees plaintext; do not describe this mode as Zero-Knowledge.
2. **HIGH (availability):** Disk/RAM pressure remains possible under anonymous
   concurrent uploads, especially 64 MiB RLNC, multipart spool duplication and
   independent verified-download spool. App admission is bounded by counts
   and a ciphertext-download budget but does not enforce a single global
   quota covering all uploads, ciphertext temp files and plaintext caches.
   Enforce container tmpfs/memory quotas and strict edge admission/rate limits,
   measure peak usage and load-test.
3. **MEDIUM (availability/retention):** Default public Blossom operators are
   external, and two successful upload receipts do not prove permanent
   storage or independent administration. Use trusted independent operators,
   monitor retrieval and failures, and test restoration.
4. **MEDIUM (assurance):** `badcrypt`, `badrlnc` and `onionguard` are security
   sensitive dependencies not independently audited as part of this review.
   `govulncheck` passing is not a comprehensive vulnerability assessment.
5. **MEDIUM (operational):** No-JS plaintext temp/cache files must reside on
   non-persistent encrypted or RAM-backed temporary storage; a process crash
   may leave files behind on persistent `/tmp`.
6. **MEDIUM (traffic):** 1,000 real mixed-mode simultaneous transfers have
   not been verified; the k6 load scripts are *harnesses*, not test results.
7. **MEDIUM (repository):** The main branch remains unprotected, so passing CI
   does not prevent an authorized actor from pushing an unreviewed change.
   Enable required PR and CI rules before treating `main` as a protected
   release source.

## Release/deployment decision

The existing `v0.2.16-rc.1` preview remains unmodified and should be regarded
as an unverified build. Merge the audit fixes only after a green CI with the
new Docker toolchain version check. Build a new preview for that specific SHA
before distributing it. Production launch still requires endpoint-level
security tests, real TLS/ingress log redaction, Tor + browser smoke tests, and
a prolonged mixed-file load test with controlled Blossom relays.

Security bulletins: https://go.dev/doc/devel/release and
https://openwall.com/lists/oss-security/2026/10/08/9.
