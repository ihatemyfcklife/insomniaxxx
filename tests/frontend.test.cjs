const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const src = fs.readFileSync('internal/app/static/boot.js', 'utf8');

function mockElement() {
  const listeners = {};
  return {
    dataset: {}, files: [], listeners, style: {},
    classList: { add() {}, remove() {} },
    addEventListener(event, listener) { listeners[event] = listener; },
    setAttribute() {},
    querySelector() { return null; },
    prepend() {}
  };
}

async function createPage({size, maxSize}) {
  const calls = []; const alerts = [];
  let readCount = 0, encryptCount = 0, qrText = null;
  const form = mockElement();
  const input = mockElement();
  input.dataset.maxUploadBytes = String(maxSize);
  input.files = [{
    name: 'fixture.txt', type:'text/plain', size,
    async arrayBuffer() { readCount++; return new Uint8Array([42,43]).buffer; }
  }];
  const ids = {uploadForm:form, file:input, wasmStatus:mockElement(),
    architectureImg:mockElement(), architectureDetailsNoJS:mockElement(),
    architectureDetailsWasm:mockElement(), modeToggle:mockElement(),
    btnModeNoJS:mockElement(), btnModeWasm:mockElement(), submitBtn:null};
  const sandbox = {
    console, Uint8Array, Blob, FormData, Number, setTimeout,
    document: {
      getElementById(id) { return ids[id] ?? null; },
      querySelector() { return null; }
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
    WebAssembly: {async instantiate(){return {instance:{}};}},
    wasmEncrypt(){encryptCount++;return {ciphertext:new Uint8Array([1,2]),hash:'a'.repeat(64),key:'secret-fragment'};},
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
  const oversized=await createPage({size:20*1024*1024,maxSize:19*1024*1024});
  let prevented=0;
  await oversized.form.onsubmit({preventDefault(){prevented++;}});
  assert.equal(prevented,1);
  assert.equal(oversized.stats().readCount,0,'oversized file must not be buffered');
  assert.equal(oversized.stats().encryptCount,0,'oversized file must not be encrypted');
  assert(oversized.alerts.some(x=>x.includes('limit')));
  assert(!oversized.calls.some(x=>x.url==='/upload-raw'));

  const classic=await createPage({size:42,maxSize:19*1024*1024});
  classic.ids.btnModeNoJS.listeners.click();
  prevented=0;
  await classic.form.onsubmit({preventDefault(){prevented++;}});
  assert.equal(prevented,1,'clearnet must never enable plaintext POST');
  assert(!classic.calls.some(x=>x.url==='/upload-raw'));

  const zk=await createPage({size:42,maxSize:19*1024*1024});
  prevented=0;
  await zk.form.onsubmit({preventDefault(){prevented++;}});
  assert.equal(prevented,1);
  assert.equal(zk.stats().encryptCount,1);
  assert(zk.stats().qrText?.includes('#secret-fragment'));
  assert.equal(zk.calls.filter(x=>x.url==='direct-relay-upload').length, 7,
    'a tiny file must upload six RLNC shards and one encrypted manifest');
  assert(!zk.calls.some(x=>x.url.includes('/qr?')||x.url.includes('secret-fragment')),
    'share keys must not appear in network URLs');
  console.log('Frontend upload-limit, mode-switch, and zero-knowledge QR network checks passed');
})().catch(err=>{console.error(err);process.exitCode=1;});
