const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const path = require('node:path');
const {createHash} = require('node:crypto');

const transport = fs.readFileSync('internal/app/static/direct-blossom.js', 'utf8');
const relays = ['https://one.example', 'https://two.example', 'https://three.example'];
const data = new Uint8Array([0x41,0x42,0x53,0x48,1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16]);
const sha = bytes => createHash('sha256').update(bytes).digest('hex');
const digest = sha(data);

function harness({offline=false}={}) {
  const calls = [];
  const root = {
    InsomniaStatic: {enabled:true,relays},
    location: {href:'https://someone.github.io/insomniaxxx/view.html?hash='+digest+'#secret',pathname:'/insomniaxxx/view.html'},
    sessionStorage: {getItem() {return '1';}}, // stale normal-server proxy selection must be ignored
    wasmSHA256:sha,
    wasmUploadAuthorization() {return 'Nostr ' + 'A'.repeat(80);}
  };
  const fetch = async (url, opts={}) => {
    calls.push({url,opts});
    assert(url.startsWith('https://'), 'GitHub Pages must not invoke Go endpoint: '+url);
    if (offline) throw new TypeError('CORS denied');
    if (url.endsWith('/upload')) {
      assert.equal(opts.method,'PUT');
      assert.equal(opts.credentials,'omit');
      assert.equal(opts.referrerPolicy,'no-referrer');
      assert.equal(opts.headers['X-SHA-256'], digest);
      return Response.json({sha256:digest,size:data.length});
    }
    assert(url.endsWith('/'+digest));
    return new Response(data, {status:200});
  };
  vm.runInNewContext(transport, {
    window:root, fetch, document:{getElementById() {return null;}},
    URL,Response,Uint8Array,TextDecoder,Blob,FormData,AbortController,
    setTimeout,clearTimeout,console
  });
  return {root,calls};
}

(async () => {
  const active=harness();
  assert.deepEqual({...await active.root.InsomniaDirect.uploadEncrypted(data,digest)},{hash:digest,direct:true});
  assert.equal(active.calls.filter(c=>c.opts.method==='PUT').length,2);
  assert.deepEqual(Array.from(await active.root.InsomniaDirect.downloadEncrypted(digest)),Array.from(data));
  assert(active.calls.every(c=>relays.some(r=>c.url.startsWith(r+'/'))));
  assert(!active.calls.some(c=>c.url.includes('#secret') || c.url.includes('insomniaxxx/view.html')));

  const down=harness({offline:true});
  await assert.rejects(down.root.InsomniaDirect.downloadEncrypted(digest),/this static site has no proxy/);
  const up=harness({offline:true});
  await assert.rejects(up.root.InsomniaDirect.uploadEncrypted(data,digest),/this static site has no proxy/);
  assert(!up.calls.some(c=>c.url==='/upload-raw'||c.url==='/direct-relays'));
  assert(!down.calls.some(c=>c.url==='/d/'+digest+'?cipher=1'));

  const dist=path.join('dist','pages');
  for (const name of ['index.html','view.html','relays.html','.nojekyll',
    'static/config.js','static/boot.js','static/view.js',
    'static/direct-blossom.js','static/app.wasm','static/wasm_exec.js']) {
    assert(fs.existsSync(path.join(dist,name)), 'missing published asset '+name);
  }
  const index=fs.readFileSync(path.join(dist,'index.html'),'utf8');
  const viewer=fs.readFileSync(path.join(dist,'view.html'),'utf8');
  const cfg=fs.readFileSync(path.join(dist,'static/config.js'),'utf8');
  assert(index.includes('src="./static/config.js"'));
  assert(index.includes('href="./relays.html"'));
  assert(!index.includes('id="proxyOnly"') && !viewer.includes('id="proxyOnly"'));
  assert(!index.includes('action="/upload"') && !viewer.includes('src="/static/'));
  assert(viewer.includes('data-cipher=""'));
  assert(cfg.includes('window.InsomniaStatic = Object.freeze('));
  console.log('GitHub Pages: static export, local relay signing, CORS failures, integrity and no Go APIs passed');
})().catch(e=>{console.error(e);process.exitCode=1;});
