const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const { createHash } = require('node:crypto');

const source = fs.readFileSync('internal/app/static/direct-blossom.js', 'utf8');
const bytes = new Uint8Array([0x41, 0x42, 0x4c, 0x4f, 1, 2, 3]);
const hash = createHash('sha256').update(bytes).digest('hex');
const relays = ['https://first.example', 'https://second.example'];
const auth = 'Nostr ' + 'A'.repeat(80);
const sha = data => createHash('sha256').update(data).digest('hex');

function harness({ badFirstDownload = false, failedUpload = false, corsFailure = false, proxyOnly = false } = {}) {
  const calls = [];
  const window = {
    location: { origin: 'https://files.example' },
    wasmSHA256: sha,
    sessionStorage: { getItem(key) { return key === 'insomnia:proxy-only' && proxyOnly ? '1' : null; } }
  };
  const response = (data, status = 200) => new Response(data, {status});
  const fetch = async (url, options = {}) => {
    calls.push({url, options});
    if (url === '/direct-relays') {
      return Response.json({ relays });
    }
    if (url === '/direct-upload-auth') {
      assert.equal(options.method, 'POST');
      assert.equal(JSON.parse(options.body).hash, hash);
      return Response.json({ targets: relays.map(url => ({url, authorization: auth})) });
    }
    if (url === '/upload-raw') {
      assert.equal(options.method, 'POST');
      return Response.json({hash});
    }
    if (url === '/d/' + hash + '?cipher=1') {
      return response(bytes);
    }
    if (url.endsWith('/upload')) {
      assert.equal(options.method, 'PUT');
      assert.equal(options.credentials, 'omit');
      assert.equal(options.headers['X-SHA-256'], hash);
      assert.equal(options.headers.Authorization, auth);
      assert.deepEqual(Array.from(options.body), Array.from(bytes));
      if (corsFailure) throw new TypeError('CORS');
      if (failedUpload && url.startsWith(relays[0])) return Response.json({sha256:'f'.repeat(64)});
      return Response.json({sha256:hash, size:bytes.length});
    }
    if (url.endsWith('/' + hash)) {
      assert.equal(options.method, 'GET');
      assert.equal(options.credentials, 'omit');
      assert.equal(options.referrerPolicy, 'no-referrer');
      if (corsFailure) throw new TypeError('CORS');
      if (badFirstDownload && url.startsWith(relays[0])) return response('malicious incorrect blob');
      return response(bytes);
    }
    throw new Error('Unexpected request ' + url);
  };
  const context = {window, fetch, document: {getElementById() { return null; }}, URL, Response, Uint8Array, TextDecoder, Blob,
    FormData, AbortController, setTimeout, clearTimeout, console};
  vm.runInNewContext(source, context);
  return { calls, api: window.InsomniaDirect };
}

(async () => {
  {
    const h = harness();
    assert.deepEqual({...await h.api.uploadEncrypted(bytes, hash)}, {hash, direct:true});
    assert.equal(h.calls.filter(x => x.options.method === 'PUT').length, 2);
    assert(!h.calls.some(x => x.url === '/upload-raw'));
    assert(!h.calls.some(x => /fragment|secret/.test(x.url)));
  }
  {
    const h = harness({ failedUpload:true });
    assert.deepEqual({...await h.api.uploadEncrypted(bytes, hash)}, {hash, direct:false});
    assert(h.calls.some(x => x.url === '/upload-raw'),
      'must fall back when fewer than two independent receipts are valid');
  }
  {
    const h = harness({ corsFailure:true });
    assert.deepEqual({...await h.api.uploadEncrypted(bytes, hash)}, {hash, direct:false});
    assert(h.calls.some(x => x.url === '/upload-raw'));
  }
  {
    const h = harness({ badFirstDownload:true });
    const result = await h.api.downloadEncrypted(hash);
    assert.deepEqual(Array.from(result), Array.from(bytes));
    assert.equal(h.calls.filter(x => x.url === '/d/' + hash + '?cipher=1').length, 0);
    assert(h.calls.filter(x => x.url.endsWith('/' + hash)).length >= 2);
  }
  {
    const h = harness({ corsFailure:true });
    const result = await h.api.downloadEncrypted(hash);
    assert.deepEqual(Array.from(result), Array.from(bytes));
    assert(h.calls.some(x => x.url === '/d/' + hash + '?cipher=1'));
  }
  {
    const h = harness({ proxyOnly:true });
    assert.deepEqual({...await h.api.uploadEncrypted(bytes, hash)}, {hash, direct:false});
    await h.api.downloadEncrypted(hash);
    assert(!h.calls.some(x => x.url === '/direct-relays' || x.url === '/direct-upload-auth'));
    assert(!h.calls.some(x => x.url.startsWith('https://')));
  }
  {
    const h = harness();
    await assert.rejects(h.api.uploadEncrypted(new Uint8Array([2,3]),hash), /SHA-256|size/);
    await assert.rejects(h.api.downloadEncrypted('bad'), /invalid/);
    assert.equal(h.calls.length, 0, 'invalid blob hashes must not initiate network fetches');
  }
  console.log('Direct Blossom: 2 receipts, corrupted relays, CORS fallback, proxy-only privacy mode, and integrity checks passed');
})().catch(err => { console.error(err); process.exitCode = 1; });
