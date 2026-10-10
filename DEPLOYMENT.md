# Production deployment checklist

This application has no file database to back up, but it does process sensitive file data and temporarily writes plaintext to local temporary files in server-side (no-JS) mode. Treat deployment configuration and logs as part of the security boundary.

## Required configuration

- Set `PUBLIC_BASE_URL` to the exact public HTTPS origin used by clients.
- Set `ONIONGUARD_COOKIE_SECURE=true` when served over HTTPS. Keep it false only for a native HTTP .onion service.
- Keep `BLOSSOM_URLS` limited to trusted, publicly reachable Blossom servers that accept generic `application/octet-stream` blobs. The application rejects private/special-use relay destinations and does not follow redirects.
- `MAX_UPLOAD_BYTES` must be positive and at most 67,108,864 bytes (64 MiB). `UPLOAD_CONCURRENCY` must be between 1 and 4. The application allows at most two concurrent download/preview operations.

## Network and reverse proxy

Terminate public TLS at a maintained reverse proxy or ingress. Forward requests to the application over loopback or a private container network, and do not expose the application port directly to the internet. Configure the proxy with upload/body limits consistent with `MAX_UPLOAD_BYTES`, connection/rate limits, and timeouts suitable for the expected file size and network (including Tor).

**Do not log full request targets or referrers.** In no-JS mode, a bearer share key is part of the URL path (`/d/<hash>/<key>`); ordinary access logs can therefore leak the key. Log a route template, HTTP method, status, and duration instead. Never log upload contents, share keys, decrypted filenames, or full share links. Ensure error monitoring does not capture request URLs.

Set `PUBLIC_BASE_URL` to the canonical origin and test redirects/cookies through the real proxy. The application does not itself terminate HTTPS.

## Container and host

The Docker image runs as the distroless non-root user. Run it with a read-only root filesystem and an ephemeral, non-executable temporary filesystem large enough for uploads and short-lived decrypted cache files. Do not mount the temporary directory persistently or back it up. Use explicit memory, CPU, PID, and disk limits; the RLNC upload path keeps intermediate buffers in memory, so provision enough memory for the configured upload size and concurrency. A practical starting point for the 64 MiB maximum is 2 GiB memory and a 1 GiB ephemeral `/tmp`, then load-test and right-size to observed peaks.

Example for a locally built image (adapt host limits and the proxy to your platform):

```sh
docker build --build-arg VERSION="$(git rev-parse --short HEAD)" -t anonblossom:release .
docker run --detach --name anonblossom \
  --read-only --tmpfs /tmp:rw,noexec,nosuid,size=1g \
  --memory=2g --cpus=2 --pids-limit=128 \
  --restart unless-stopped --env-file .env \
  -p 127.0.0.1:8080:8080 anonblossom:release
```

Do not put secrets directly in image layers or Docker build arguments. Keep environment files readable only by the deployment identity.

## Privacy and service guarantees

- **No-JS mode is not zero-knowledge:** the server sees plaintext and the share key during upload/download, and decrypted data can exist briefly in `/tmp`.
- **WASM mode** encrypts/decrypts client-side only when the browser actually loads and runs the WASM flow. Test that mode explicitly; do not infer it from a successful homepage response.
- The application has no database or upload index. Public Blossom operators control the persistence and availability of stored ciphertext. There is no application-level revocation or backup for objects stored on those external relays.
- The application and relays see network and blob metadata. Do not promise anonymity or resistance to traffic analysis.

## Security migration after the manifest v2 fix

Manifest v1 and file encryption previously used the same share key with a reset ChaCha20-Poly1305 nonce counter. Version 2 manifests now use a domain-separated key. Older v1 manifest links remain decryptable for compatibility, **not** cryptographically repaired. Treat previously shared sensitive RLNC files as potentially exposed: re-upload those files after deploying the fix and redistribute their new links. Rotating the service binary alone cannot fix existing ciphertext stored on external Blossom relays.

The Zero-Knowledge QR code is generated inside the browser. Verify with browser developer tools that no request to `/qr?data=` carries a fragment-bearing share link. For all deployments, redact request paths, query strings and referrers from every proxy, CDN, WAF, and monitoring service.

## Release gate

Before production traffic, all of the following should be evidenced for the exact release commit:

1. CI passes formatting, `go test -race ./...`, `go vet ./...`, `govulncheck`, the Linux server build, the WASM build, and the Docker build.
2. Run a staging smoke test through the real TLS proxy for upload, preview, download, expiry/cache cleanup, the WASM zero-knowledge flow, and the no-JS flow.
3. Test with the maximum allowed upload size, simultaneous uploads/downloads, relay timeouts/failures, malformed and oversized relay responses, and graceful SIGTERM shutdown.
4. Confirm HTTPS and cookie behavior; verify proxy/application logs redact bearer keys and referrers; verify disk/memory/CPU limits, alerts, and recovery/redeployment procedures.
5. Review any vulnerability scan findings and obtain an independent security review appropriate to the threat model before advertising this service as secure or anonymous.

CI and code-level tests do not prove that a particular deployment is safe. A release is not production-approved until the staging checks, resource sizing, logging controls, and actual HTTPS deployment have been verified.
