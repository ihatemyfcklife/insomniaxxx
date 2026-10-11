const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const { createHash } = require('node:crypto');

const bytes = (text) => Uint8Array.from(Buffer.from(text)).buffer;
const digest = (value) => createHash('sha256').update(Buffer.from(value)).digest('hex');
const cipher = bytes('synthetic ciphertext');
const plain = bytes('synthetic plaintext');
const share = `/d/${'a'.repeat(64)}/${'b'.repeat(43)}`;

function load(script, http) {
  const results = [];
  const context = vm.createContext({
    ArrayBuffer,
    __ENV: {
      K6_BASE_URL: 'https://staging.example', K6_BLOB_HASH: digest(cipher),
      K6_NOJS_SHARE_PATH: share, K6_CIPHER_FILE: 'cipher', K6_NOJS_FILE: 'plain',
      K6_VUS: '4',
    },
    crypto: { sha256: digest }, http,
    open: (path) => path === 'cipher' ? cipher : plain,
    sleep: () => {},
    check: (response, checks) => {
      for (const fn of Object.values(checks)) results.push(fn(response));
    },
  });
  const source = fs.readFileSync(script, 'utf8')
    .replace(/^import .*;$/gm, '')
    .replace('export default function ()', 'function run()')
    .replace(/export /g, '');
  vm.runInContext(source, context);
  return { context, results };
}

for (const body of [cipher, bytes('<html>Challenge</html>'), bytes('corrupt'), null]) {
  const { context, results } = load('tests/load/cipher-download.mjs', {
    get: () => ({ status: 200, body }),
  });
  context.run();
  assert.deepEqual(results, [body === cipher]);
  assert.equal(vm.runInContext('options.systemTags.includes("url")', context), false);
}

for (const valid of [true, false]) {
  let roundtrips = 0;
  const { context, results } = load('tests/load/mixed-transfers.mjs', {
    file: (data) => data,
    get: (url) => {
      if (url.includes(share)) roundtrips++;
      return { status: 200, body: valid
        ? (url.includes('?cipher=1') ? cipher : plain)
        : bytes('<html>Challenge</html>') };
    },
    post: (url) => ({
      status: 200,
      body: valid ? `<input value="https://staging.example${share}">` : '<html>Challenge</html>',
      json: () => {
        if (!valid) throw new Error('not JSON');
        return { hash: digest(cipher) };
      },
    }),
  });
  context.downloadCipher();
  context.downloadNoJS();
  context.uploadCipher();
  context.uploadNoJS();
  assert.deepEqual(results, Array(valid ? 5 : 4).fill(valid));
  assert.equal(roundtrips, valid ? 2 : 1);
}
console.log('Load checks reject HTTP 200 challenges/corruption and verify upload roundtrips');
