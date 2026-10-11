const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const {createHash} = require('node:crypto');

const transport = fs.readFileSync('internal/app/static/direct-blossom.js','utf8');
const stream = fs.readFileSync('internal/app/static/stream-file.js','utf8');
const relays = ['https://first.example','https://second.example','https://third.example'];
const nostr = 'Nostr ' + 'A'.repeat(80);
const hashBytes = bytes => createHash('sha256').update(bytes).digest('hex');
const shards = Array.from({length:17}, (_,i) =>
  new Uint8Array([0x41,0x42,0x53,0x48,...Array.from({length:32},(_,j)=>(i+j)%256)]));

function harness({invalidBatch=false,proxyOnly=false}={}) {
  const calls = [];
  const window = {
    location:{origin:'https://files.example',pathname:'/'},
    wasmSHA256:hashBytes,
    wasmUploadAuthorization: () => invalidBatch ? null : nostr,
    sessionStorage:{getItem(k){return proxyOnly && k==='insomnia:proxy-only'?'1':null;}}
  };
  const fetch = async (url, opts={}) => {
    calls.push({url,opts});
    if (url==='/direct-relays') return Response.json({relays});
    if (url==='/direct-upload-auth/batch') {
      const req = JSON.parse(opts.body);
      assert(req.blobs.length>=1 && req.blobs.length<=8);
      assert.equal(opts.credentials,'same-origin');
      assert(!opts.body.includes('key'), 'never send share keys with grants');
      if (invalidBatch) return Response.json({grants:[]});
      return Response.json({grants:req.blobs.map(blob=>({
        hash:blob.hash,
        targets:relays.map(r=>({url:r,authorization:nostr}))
      }))});
    }
    if (url==='/direct-upload-auth') {
      return Response.json({targets:relays.map(r=>({url:r,authorization:nostr}))});
    }
    if (url==='/upload-raw') {
      const blob=opts.body.get('file');
      return Response.json({hash:hashBytes(new Uint8Array(await blob.arrayBuffer()))});
    }
    if (url.endsWith('/upload')) {
      assert.equal(opts.credentials,'omit');
      assert.equal(opts.referrerPolicy,'no-referrer');
      assert.equal(opts.headers.Authorization,nostr);
      assert.equal(hashBytes(opts.body),opts.headers['X-SHA-256']);
      return Response.json({sha256:opts.headers['X-SHA-256'],size:opts.body.byteLength});
    }
    throw new Error('unexpected network request: '+url);
  };
  const sandbox={window,fetch,document:{getElementById(){return null;}},
    URL,Response,Uint8Array,Blob,FormData,TextDecoder,AbortController,
    setTimeout,clearTimeout,console};
  vm.runInNewContext(transport,sandbox);
  vm.runInNewContext(stream,sandbox);
  return {calls,window};
}

(async()=>{
  const h=harness();
  const hashes=await h.window.InsomniaStream.uploadShards(shards,
    (bytes,hash,index)=>h.window.InsomniaDirect.uploadEncrypted(bytes,hash,{shardIndex:index}));
  assert.equal(hashes.length,17);
  assert.deepEqual(Array.from(hashes),shards.map(hashBytes));
  assert.equal(h.calls.filter(c=>c.url==='/direct-upload-auth/batch').length,0,
    'all relay signatures must be produced locally');
  assert.equal(h.calls.filter(c=>c.url==='/direct-upload-auth').length,0,
    'a successful batch must avoid per-shard signing calls');
  assert.equal(h.calls.filter(c=>c.url==='/upload-raw').length,0);
  assert.equal(h.calls.filter(c=>c.opts.method==='PUT').length,34,
    'two independent relay receipts per encrypted shard');
  const h2=harness({invalidBatch:true});
  await assert.rejects(h2.window.InsomniaStream.uploadShards(shards,
    (bytes,hash,index)=>h2.window.InsomniaDirect.uploadEncrypted(bytes,hash,{shardIndex:index})), /Direct upload failed/);
  assert(!h2.calls.some(c => c.url.startsWith('/direct-upload-auth') || c.url === '/upload-raw'));
  const h3=harness({proxyOnly:true});
  await h3.window.InsomniaStream.uploadShards(shards,
    (bytes,hash,index)=>h3.window.InsomniaDirect.uploadEncrypted(bytes,hash,{shardIndex:index}));
  assert.equal(h3.calls.filter(c=>c.url==='/direct-relays'||c.url.startsWith('https://')).length,0,
    'explicit proxy-only mode must never contact storage relays');
  assert.equal(h3.calls.filter(c=>c.url==='/upload-raw').length,17);
  console.log('Blossom batch auth: 17 locally signed shards, 2 replicas, fail-closed and proxy mode passed');
})().catch(e=>{console.error(e);process.exitCode=1});
