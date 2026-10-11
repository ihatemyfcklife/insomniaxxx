const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const src = fs.readFileSync('internal/app/static/boot.js', 'utf8');

function mockElement() {
  const listeners = {};
  const classes = new Set();
  return {
    dataset: {}, files: [], listeners, style: {},
    classList: {
      add(name) { classes.add(name); },
      remove(name) { classes.delete(name); },
      contains(name) { return classes.has(name); }
    },
    addEventListener(event, listener) { listeners[event] = listener; },
    setAttribute() {},
    querySelector() { return null; },
    prepend() {},
    appendChild() {}
  };
}

async function createPage({size, maxSize, wasmFailure=false, fileSelected=true, encryptFailure=false}) {
  const calls = []; const alerts = [];
  let readCount = 0, encryptCount = 0, qrText = null;
  const form = mockElement();
  const input = mockElement();
  input.dataset.maxUploadBytes = String(maxSize);
  input.files = fileSelected ? [{
    name: 'fixture.txt', type:'text/plain', size,
    async arrayBuffer() { readCount++; return new Uint8Array([42,43]).buffer; }
  }] : [];
  const submit = mockElement();
  submit.disabled = true;
  const submitText = mockElement();
  const loading = mockElement();
  const mascot = mockElement();
  submit.querySelector = (selector) => ({
    '.btn-text': submitText,
    '.btn-loading': loading,
    'img.upload-btn-mascot': mascot
  })[selector] || null;
  const ids = {uploadForm:form, file:input, wasmStatus:mockElement(), submitBtn:submit};
  const sandbox = {
    console, Uint8Array, Blob, FormData, Number, setTimeout,
    document: {
      getElementById(id) { return ids[id] ?? null; },
      querySelector() { return null; },
      createElement() { return mockElement(); },
      createTextNode(text) { return { textContent: text }; }
    },
    window: {location:{origin:'https://files.example'}, InsomniaDirect:{
      async uploadEncrypted(bytes, hash) {calls.push({url:'direct-relay-upload'}); return {hash};}
    }},
    TextEncoder,
    wasmSHA256(){return 'a'.repeat(64);},
    wasmEncodeRLNC(_ciphertext,k,n){
      assert.equal(k,4, 'small files must use K=4');
      assert.equal(n,6, 'small files must have 6 RLNC shards');
      return Array.from({length:n},()=>new Uint8Array(80));
    },
    wasmEncryptWithKey(){return {ciphertext:new Uint8Array([6,7]),hash:'a'.repeat(64)};},
    URL: {createObjectURL() {return 'blob:local-qr';}},
    alert(text) {alerts.push(text);},
    Go: class {constructor(){this.importObject={};} run(){}},
    WebAssembly: {async instantiate(){if(wasmFailure) throw new Error("failed"); return {instance:{}};}},
    wasmEncrypt(){
      encryptCount++;
      if (encryptFailure) return {error:'encryption failed'};
      return {ciphertext:new Uint8Array([1,2]),hash:'a'.repeat(64),key:'secret-fragment'};
    },
    wasmGenerateQR(share){qrText=share;return new Uint8Array([137,80,78,71]);},
    async fetch(url,options) {
      calls.push({url,options});
      if(url==='/static/app.wasm')return {ok:true,async arrayBuffer(){return new ArrayBuffer(0);}};
      if(url==='/upload-raw')return {ok:true,async json(){return {hash:'a'.repeat(64)};}};
      throw new Error('unexpected network request '+url);
    }
  };
  vm.runInNewContext(src,sandbox);
  // Boot initialization awaits the mocked WASM loader.
  for(let i=0;i<10;i++) await new Promise(resolve=>setTimeout(resolve,0));
  assert.equal(typeof form.onsubmit,'function','WASM upload handler not installed');
  return {form,ids,calls,alerts,stats:()=>({readCount,encryptCount,qrText})};
}

(async()=>{
  const failed=await createPage({size:42,maxSize:1024,wasmFailure:true});
  let blocked=0;
  await failed.form.onsubmit({preventDefault(){blocked++;}});
  assert.equal(blocked,1,'failed WASM must not enable plaintext submission');
  assert.equal(failed.stats().readCount,0);
  assert(!failed.calls.some(x=>x.url==='/upload-raw'));

  const empty=await createPage({size:42,maxSize:1024,fileSelected:false});
  assert.equal(empty.ids.submitBtn.disabled,false,'successful Wasm startup enables uploads');
  assert.equal(empty.ids.submitBtn.classList.contains('is-loading'),false,'idle upload must not show progress');
  await empty.form.onsubmit({preventDefault(){}});
  assert.equal(empty.ids.submitBtn.classList.contains('is-loading'),false,'no-file submission must not show progress');
  assert.equal(empty.stats().encryptCount,0);

  const failedEncrypt=await createPage({size:42,maxSize:1024,encryptFailure:true});
  await failedEncrypt.form.onsubmit({preventDefault(){}});
  assert.equal(failedEncrypt.ids.submitBtn.classList.contains('is-loading'),false,'failed upload must reset progress state');
  assert.equal(failedEncrypt.ids.submitBtn.disabled,false,'failed upload must re-enable button');

  const oversized=await createPage({size:20*1024*1024,maxSize:19*1024*1024});
  let prevented=0;
  await oversized.form.onsubmit({preventDefault(){prevented++;}});
  assert.equal(prevented,1);
  assert.equal(oversized.stats().readCount,0,'oversized file must not be buffered');
  assert.equal(oversized.stats().encryptCount,0,'oversized file must not be encrypted');
  assert(oversized.alerts.some(x=>x.includes('limit')));
  assert(!oversized.calls.some(x=>x.url==='/upload-raw'));

  const zk=await createPage({size:42,maxSize:19*1024*1024});
  prevented=0;
  const upload = zk.form.onsubmit({preventDefault(){prevented++;}});
  assert.equal(zk.ids.submitBtn.classList.contains('is-loading'),true,'upload shows progress only after submit');
  await upload;
  assert.equal(prevented,1);
  assert.equal(zk.stats().encryptCount,1);
  assert(zk.stats().qrText?.includes('#secret-fragment'));
  assert.equal(zk.calls.filter(x=>x.url==='direct-relay-upload').length, 7,
    'a tiny file must upload six RLNC shards and one encrypted manifest');
  assert(!zk.calls.some(x=>x.url.includes('/qr?')||x.url.includes('secret-fragment')),
    'share keys must not appear in network URLs');
  console.log('Frontend upload-limit, loading-state, and zero-knowledge QR network checks passed');
})().catch(err=>{console.error(err);process.exitCode=1;});
