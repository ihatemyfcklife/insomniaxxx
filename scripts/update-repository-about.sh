#!/usr/bin/env bash
# Update the GitHub repository About description, website and topic list.
# Requires: GitHub CLI authenticated as a user with repository admin access.
set -euo pipefail

repo="ihatemyfcklife/insomniaxxx"
description="Privacy-focused file sharing with Go/WASM, RLNC and Blossom relays. Zero-Knowledge browser encryption, Tor No-JS and GitHub Pages."
homepage="https://ihatemyfcklife.github.io/insomniaxxx/"

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
topics="$root/.github/repository-topics.json"

if ! command -v gh >/dev/null 2>&1; then
  echo "GitHub CLI (gh) is required: https://cli.github.com/" >&2
  exit 1
fi
if ! gh auth status >/dev/null 2>&1; then
  echo "Authenticate to GitHub first with: gh auth login" >&2
  exit 1
fi
test -s "$topics" || { echo "Missing topics: $topics" >&2; exit 1; }

# Never overwrite repository settings with an empty or invalid topics document.
if command -v python3 >/dev/null 2>&1; then
  python3 - "$topics" <<'PY'
import json, re, sys
names = json.load(open(sys.argv[1], encoding="utf-8"))["names"]
assert 1 <= len(names) <= 20 and len(names) == len(set(names))
assert all(isinstance(name, str) and re.fullmatch(r"[a-z0-9-]{1,50}", name) for name in names)
PY
fi

gh repo edit "$repo" --description "$description" --homepage "$homepage"
gh api --method PUT "repos/$repo/topics" --input "$topics" >/dev/null

printf 'Updated %s About, homepage and topics.\n' "$repo"
printf 'Review: https://github.com/%s\n' "$repo"
