# Production release controls

This checklist is for repository administrators and release operators. It is **not** proof that a public deployment has passed security review. Also follow [production deployment checklist](../README.md#production-deployment-checklist).

## Protect `main` (repository administrator action)

GitHub repository **Settings → Rules → Rulesets** (or **Settings → Branches → Branch protection rules**):

1. Create an **active branch ruleset** targeting `main` (or an equivalent branch-protection rule).
2. **Require a pull request before merging**, including at least one independent approval when a reviewer is available. Dismiss stale approvals on new pushes and require resolved review conversations.
3. **Require status checks to pass before merging**. Choose the actual `CI / test-and-build` check (the job is `test-and-build`, from `.github/workflows/ci.yml`; choose the GitHub Actions check source if prompted). Require branches to be up to date before merging.
4. **Block force pushes and branch deletion**. Apply the rule to administrators; do not configure broad bypass actors. Avoid allowing direct pushes to `main`.
5. Save the rule and verify it is **Active**. Test it with a PR that has not yet passed the required CI check. A passing build alone does not enforce the rule.

> The connected GitHub App cannot change branch protection because it does not have Administration write permission. These settings must be applied by a repository administrator in GitHub.

## Confirm the exact code being published

As of 2026-10-10, `v0.2.15` points to `ff2757168f80856bb5213b174ecee8b9c7f6a16e` and contains the security fixes that postdate `v0.2.14`. Future releases must be checked against the exact CI-verified `main` SHA; some commits intentionally contain `[skip release]`.

Never remove those skip instructions merely to cause an automatic release. A successful CI run on `main` does **not** rebuild previous GitHub Release assets.

## Stage and manually publish a skipped security update

1. Finish and record the staging checks listed in [production deployment checklist](../README.md#production-deployment-checklist): real TLS/Tor ingress, no-JS and WASM upload/download/recovery, CAPTCHA/fragment privacy, no URL-key logging, max-size and concurrent transfers, relay failure handling, temporary-file cleanup, memory/CPU/disk limits, and security review.
2. Ensure the selected `main` SHA has a successful `CI / test-and-build` check, and confirm the required branch rule is active.
3. In **Actions → release → Run workflow**, select `main`. For an intentionally skipped commit, enable **release_skipped** and **staging_verified**. Select `bump=patch` unless a reviewed versioning decision requires otherwise. These inputs are an explicit release attestation, **not** automated staging tests.
4. The workflow refuses to release if `main` moved since it started, if it cannot locate successful CI for the exact SHA, or if a skipped commit is not manually approved. It builds WASM and Linux amd64/arm64 binaries, checks their SHA-256 digests, then creates the new tag and GitHub Release.
5. Verify that the GitHub Release tag resolves to the approved commit and all five assets are present: `anonblossom-linux-amd64`, `anonblossom-linux-arm64`, `app.wasm`, `wasm_exec.js`, and `SHA256SUMS.txt`. Verify checksums with `sha256sum -c SHA256SUMS.txt` from the downloaded asset directory.
6. Deploy **that release** to staging/production through the approved deployment path, monitoring post-deployment health. Keep a rollback procedure; a released binary is not automatically deployed.

If publication stops after tag creation, inspect the workflow and release assets before retrying. Do not move or overwrite an existing release tag.

## Historical cryptographic caution

Older v1 RLNC manifest links cannot be repaired in place by releasing new software. Re-upload sensitive affected files and distribute their new links as described in [production deployment checklist](../README.md#production-deployment-checklist).
