const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');

const src = fs.readFileSync('internal/app/static/stream-file.js', 'utf8');
const hash = 'a'.repeat(64);
const window = {};
const progress = [];
let maxRead = 0;
let readTotal = 0;
let fullFileReads = 0;
let streamFinished = false;
let streamAborted = false;

const fileSize = 3 * 1024 * 1024 + 17;
const file = {
  name: 'large-file.bin', type: 'application/octet-stream', size: fileSize,
  async arrayBuffer() { fullFileReads++; throw new Error('must not read full plaintext'); },
  slice(start, end) {
    const len = Math.min(end, fileSize) - start;
    assert(len >= 0 && len <= 1024 * 1024);
    return {
      async arrayBuffer() {
        readTotal += len;
        maxRead = Math.max(maxRead, len);
        return new Uint8Array(len).buffer;
      }
    };
  }
};

const sandbox = {
  window, Blob, Uint8Array, Array, Error, Math, Number, Promise, console,
  wasmEncryptStreamStart() {
    return { id: 5, key: 'local-fragment-key', header: new Uint8Array(16) };
  },
  wasmEncryptStreamPush(id, chunk) {
    assert.equal(id, 5);
    assert(chunk.length <= 1024 * 1024);
    return [new Uint8Array([42])];
  },
  wasmEncryptStreamFinish(id) {
    assert.equal(id, 5);
    streamFinished = true;
    return {chunks: [new Uint8Array([43])], hash};
  },
  wasmEncryptStreamAbort(id) {assert.equal(id, 5); streamAborted = true;},
  wasmSHA256(data) {return Number(data[0]).toString(16).padStart(64, "0");}
};
Object.assign(window, {
  wasmEncryptStreamStart: sandbox.wasmEncryptStreamStart,
  wasmEncryptStreamPush: sandbox.wasmEncryptStreamPush,
  wasmEncryptStreamFinish: sandbox.wasmEncryptStreamFinish,
  wasmEncryptStreamAbort: sandbox.wasmEncryptStreamAbort,
  wasmSHA256: sandbox.wasmSHA256
});
vm.runInNewContext(src, sandbox);

(async () => {
  const result = await window.InsomniaStream.encryptFile(file, (done, total) => progress.push({done, total}));
  assert.equal(result.key, 'local-fragment-key');
  assert.equal(result.hash, hash);
  assert(result.ciphertext instanceof Blob);
  assert.equal(result.ciphertext.size, 16 + Math.ceil(fileSize / (1024 * 1024)) + 1);
  assert.equal(maxRead, 1024 * 1024);
  assert.equal(readTotal, fileSize);
  assert.equal(fullFileReads, 0, 'file.arrayBuffer() must not be called');
  assert.equal(streamFinished, true);
  assert.equal(streamAborted, false);
  assert.equal(progress.at(-1).done, fileSize);

  const shards = Array.from({length: 9}, (_, i) => new Uint8Array([i+1, i+2]));
  let active = 0;
  let maxActive = 0;
  const resolved = await window.InsomniaStream.uploadShards(shards, async (bytes, hash) => {
    active++;
    maxActive = Math.max(active, maxActive);
    await new Promise(resolve => setTimeout(resolve, (9 - bytes[0]) * 2));
    active--;
    return {hash};
  }, null, 3);
  assert.equal(maxActive, 3);
  assert.deepEqual(Array.from(resolved), shards.map(s => Number(s[0]).toString(16).padStart(64, "0")),
    'manifest hashes must follow original shard order, not completion order');

  let uploaded = 0;
  await assert.rejects(window.InsomniaStream.uploadShards(shards, async (bytes, sha) => {
    uploaded++;
    if (bytes[0] === 2) throw new Error('relay unavailable');
    return {hash: sha};
  }, null, 3), /relay unavailable/);
  assert(uploaded < shards.length, 'failed shards must stop new work');
  await assert.rejects(window.InsomniaStream.uploadShards(shards, async () => ({hash:'wrong'})),
    /receipt mismatch/);

  console.log('WASM bounded plaintext slice encryption and ordered, bounded RLNC shard workers passed');
})().catch(err => {console.error(err);process.exitCode=1;});
