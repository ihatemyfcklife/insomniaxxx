const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const path = require('node:path');
const {createHash} = require('node:crypto');

const transport = fs.readFileSync('internal/app/static/direct-blossom.js', 'utf8');
const relays = ['https://one.example', 'https://two.example', 'https://three.example',
  'https://four.example', 'https://five.example'];
const data = new Uint8Array([0x41,0x42,0x53,0x48,1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16]);
const sha = bytes => createHash('sha256').update(bytes).digest('hex');
const digest = sha(data);

function harness({offline=false,failedRelays=[]}={}) {
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
    if (offline || failedRelays.some(relay=>url.startsWith(relay+'/')))
      throw new TypeError('CORS denied');
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

  // Static transfers must be able to fail over beyond the old three-relay cap,
  // without ever contacting a directory-only server.
  const fallback=harness({failedRelays:relays.slice(0,3)});
  assert.deepEqual({...await fallback.root.InsomniaDirect.uploadEncrypted(data,digest)},
    {hash:digest,direct:true});
  assert(fallback.calls.some(c=>c.url.startsWith(relays[4]+'/upload')));
  assert(fallback.calls.every(c=>relays.some(relay=>c.url.startsWith(relay+'/'))));
  assert.deepEqual(Array.from(await fallback.root.InsomniaDirect.downloadEncrypted(digest)),Array.from(data));

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
  const relaysPage=fs.readFileSync(path.join(dist,'relays.html'),'utf8');
  const cfg=fs.readFileSync(path.join(dist,'static/config.js'),'utf8');

  const version=index.match(/<span class="app-version">(v[0-9]+\.[0-9]+\.[0-9]+)<\/span>/)?.[1];
  assert(version, 'static site must display a real released version instead of dev');
  for (const page of [index, viewer, relaysPage]) {
    assert(page.includes('href="https://ihatemyfcklife.github.io/insomniaxxx/" class="logo-link"'),
      'every static page logo must point to the project homepage');
    assert(!page.includes('href="/" class="logo-link"') &&
      !page.includes('href="./" class="logo-link"'),
      'logo must not link to domain root or a relative path');
    assert(page.includes('<span class="app-version">'+version+'</span>'),
      'all exported pages must show the same release version');
    for (const repo of ['insomniaxxx', 'badcrypt', 'badrlnc', 'onionguard']) {
      assert(page.includes('https://github.com/ihatemyfcklife/'+repo),
        'static footer missing source link for '+repo);
    }
    assert(!page.includes('>PGP</a>') && !page.includes('>Contact</a>') &&
      !page.includes('>ads</a>'), 'old placeholder links must not ship');
  }
  assert(relaysPage.includes('class="relays-card"'),
    'static relay page must reuse the regular relay card');
  assert(relaysPage.includes('class="relays-table" role="grid"'),
    'static relay page must reuse the desktop table');
  const match=cfg.match(/^window\.InsomniaStatic = Object\.freeze\((.*)\);/s);
  assert(match,'the static site must include its trusted Blossom config');
  const activeRelays=JSON.parse(match[1]).relays;
  assert(activeRelays.length >= 2 && activeRelays.length <= 12);
  assert.equal((relaysPage.match(/class="relay-row"/g)||[]).length,
    activeRelays.length + (relaysPage.match(/>Directory<\/small>/g)||[]).length,
    'every active or directory relay must have a responsive table row');
  assert.equal((relaysPage.match(/>Configured<\/small>/g)||[]).length,
    activeRelays.length,'every configured storage relay must appear in the table');
  assert((relaysPage.match(/>Directory<\/small>/g)||[]).length >= 3,
    'the public Blossom directory must display multiple additional services');
  assert.equal((relaysPage.match(/class="relay-badge unchecked">Not checked<\/span>/g)||[]).length,
    (relaysPage.match(/class="relay-row"/g)||[]).length,
    'never claim an unmonitored Blossom server is online');
  assert(relaysPage.includes('Configured storage relays and public Blossom directory.'));
  assert(relaysPage.includes('Directory entries are reference-only'));
  assert(relaysPage.includes('Only entries marked Configured are used for direct transfers.'));
  assert(relaysPage.includes('<th scope="col">Source</th>') &&
         relaysPage.includes('<th scope="col">Last Check</th>'));
  assert(!relaysPage.includes('Auto-discovered via Nostr') &&
         !relaysPage.includes('class="relay-badge online"') &&
         !relaysPage.includes('class="relay-badge offline"'));
  assert(!relaysPage.includes('href="/relays"'),
    'static relay page must not navigate to a Go-only route');
  assert(index.includes('src="./static/config.js"'));
  assert(index.includes('href="./relays.html"'));
  assert(!index.includes('id="proxyOnly"') && !viewer.includes('id="proxyOnly"'));
  assert(!index.includes('action="/upload"') && !viewer.includes('src="/static/'));
  assert(viewer.includes('data-cipher=""'));
  assert(cfg.includes('window.InsomniaStatic = Object.freeze('));
  console.log('GitHub Pages: static export, local relay signing, CORS failures, integrity and no Go APIs passed');
})().catch(e=>{console.error(e);process.exitCode=1;});
