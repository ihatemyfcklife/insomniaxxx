const assert = require('node:assert/strict');
const fs = require('node:fs');

const readme = fs.readFileSync('README.md', 'utf8');
const shell = fs.readFileSync('scripts/update-repository-about.sh', 'utf8');
const json = JSON.parse(fs.readFileSync('.github/repository-topics.json', 'utf8'));

assert(Array.isArray(json.names));
assert(json.names.length >= 8 && json.names.length <= 20);
assert.equal(json.names.length, new Set(json.names).size, 'GitHub topics must be unique');
for (const name of json.names) {
  assert.match(name, /^[a-z0-9-]{1,50}$/, 'invalid GitHub topic');
}
for (const expected of [
  'blossom', 'file-sharing', 'golang', 'tor', 'wasm', 'nostr',
  'rlnc', 'github-pages', 'zero-knowledge'
]) {
  assert(json.names.includes(expected), 'missing repository topic ' + expected);
}
for (const text of [
  'https://ihatemyfcklife.github.io/insomniaxxx/',
  '## Choose an edition',
  'GitHub Pages (static)',
  'Clearnet (Go-hosted)',
  'Tor / No-JS',
  '## Repository About and topics',
  'bash scripts/update-repository-about.sh',
  'do **not**',
]) {
  assert(readme.includes(text), 'README missing ' + text);
}
assert(shell.includes('gh repo edit "$repo"'), 'missing GitHub About update');
assert(shell.includes('gh api --method PUT "repos/$repo/topics"'),
  'missing GitHub topics update');
assert(shell.includes('repo="ihatemyfcklife/insomniaxxx"'));
assert(shell.includes('description="Privacy-focused file sharing'));
assert(shell.includes('homepage="https://ihatemyfcklife.github.io/insomniaxxx/"'));
console.log('README and repository metadata topics/CLI workflow checks passed');
