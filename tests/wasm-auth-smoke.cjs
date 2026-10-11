// Exercise the built WASM export, not a mocked signer.
const fs = require('node:fs');
const vm = require('node:vm');
const assert = require('node:assert/strict');
globalThis.crypto = require('node:crypto').webcrypto;
vm.runInThisContext(fs.readFileSync('internal/app/static/wasm_exec.js', 'utf8'));
(async () => {
  const go = new Go();
  const { instance } = await WebAssembly.instantiate(
    fs.readFileSync('internal/app/static/app.wasm'), go.importObject);
  go.run(instance);
  const hash = 'a'.repeat(64);
  const token = wasmUploadAuthorization('https://relay.example', hash);
  assert.equal(typeof token, 'string');
  assert(token.startsWith('Nostr '));
  const event = JSON.parse(Buffer.from(token.slice(6), 'base64').toString());
  assert.equal(event.kind, 24242);
  const serialized = JSON.stringify([0, event.pubkey, event.created_at, event.kind, event.tags, event.content]);
  const expectedID = require('node:crypto').createHash('sha256').update(serialized).digest('hex');
  assert.equal(event.id, expectedID, 'WASM event ID must use canonical Nostr serialization');
  assert(event.tags.some(t => t[0] === 'x' && t[1] === hash));
  assert(event.tags.some(t => t[0] === 'server' && t[1] === 'relay.example'));
  assert.match(event.sig, /^[0-9a-f]{128}$/);
  assert(wasmUploadAuthorization('https://relay.example', 'bad').error);
  console.log('Real WASM local relay authorization passed');
  process.exit(0);
})().catch(error => { console.error(error); process.exit(1); });
