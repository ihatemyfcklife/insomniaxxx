const assert = require('node:assert/strict');
const vm = require('node:vm');
const fs = require('node:fs');
const saved = new Map();
const sessionStorage = { getItem: k => saved.get(k) ?? null, setItem: (k,v) => saved.set(k,v), removeItem: k => saved.delete(k) };
const hash = 'a'.repeat(64), secret = 'A'.repeat(43);
const pathname = '/d/' + hash;
const gate = fs.readFileSync('internal/app/static/zk-fragment.js','utf8');
vm.runInNewContext(gate,{window:{location:{pathname,hash:'#'+secret}},sessionStorage,Date});
assert(saved.has('insomnia:pending-zk:'+pathname));
const view = fs.readFileSync('internal/app/static/view.js','utf8');
const history = {replaceState(_s,_t,url){assert.equal(url,pathname+'#'+secret);}};
const proxyOptions={style:{}};
let errorRecoveryVisible=false;
const loader={style:{},appendChild(node){errorRecoveryVisible = node === proxyOptions;}};
const context={window:{location:{pathname,hash:'',search:''},history},sessionStorage,Date,
  document:{currentScript:{getAttribute(){return '/d/'+hash+'?cipher=1';}},getElementById(id){
    if(id==='zkLoader') return loader;
    if(id==='zkProxyOptions') return proxyOptions;
    return {style:{},textContent:''};
  }},
  WebAssembly: undefined};
vm.runInNewContext(view,context);
assert(errorRecoveryVisible,'failed Zero-Knowledge loading must expose the existing proxy checkbox for retry');
assert(!saved.has('insomnia:pending-zk:'+pathname),'stored key must be removed after restoration');
const noSecret = {window:{location:{pathname,hash:'#not-a-key'}},sessionStorage,Date};
vm.runInNewContext(gate,noSecret);
assert(!saved.has('insomnia:pending-zk:'+pathname),'invalid fragments must not be stored');
// The final DOM position matters: server-side template ordering alone cannot
// catch a checkbox left above the preview by a previous loading/error state.
async function testProxyPositionAfterDecryption() {
  const positions = ['proxy', 'header', 'preview', 'buttons'];
  const fallbackChildren = [];
  let fallbackRoot = null;
  const checkboxArea = {style:{}};
  const preview = {
    insertAdjacentElement(position, element) {
      assert.equal(position, 'afterend');
      assert.equal(element, checkboxArea);
      positions.splice(positions.indexOf('proxy'), 1);
      positions.splice(positions.indexOf('preview') + 1, 0, 'proxy');
    },
    replaceChildren(node) {fallbackRoot = node;}
  };
  const content = {style:{display:'none'}};
  const download = {style:{display:'none'},setAttribute() {}};
  const elements = {
    zkLoader:{style:{}}, zkContent:content, zkSpinner:{style:{}},
    zkVisual:{style:{}}, zkStatusTitle:{style:{}}, zkStatus:{style:{}},
    previewBox:preview, zkDownload:download, zkProxyOptions:checkboxArea,
    previewName:{}, previewSize:{}, previewType:{}
  };
  const browser = {
    location:{pathname, hash:'#'+secret, search:''},
    InsomniaDirect:{async downloadEncrypted() {return new Uint8Array([42]);}}
  };
  const sandbox = {
    window:browser, sessionStorage, Date, Blob, TextDecoder, Uint8Array,
    URL:{createObjectURL(){return 'blob:recovered-file';}},
    document:{
      title:'download · insomniaXXX',
      currentScript:{getAttribute(){return pathname+'?cipher=1';}},
      getElementById(id){return elements[id] ?? null;},
      createElement(tag) {return {
        tag, className:'', textContent:'',
        appendChild(node){fallbackChildren.push(node);}
      };}
    },
    Go:class {constructor(){this.importObject={};} run(){}},
    WebAssembly:{async instantiate(){return {instance:{}};}},
    fetch:async()=>({ok:true,async arrayBuffer(){return new ArrayBuffer(0);}}),
    wasmDecrypt:()=>({
      data:new Uint8Array([42]), name:'tiny.bin',
      type:'application/octet-stream', size:1
    })
  };
  vm.runInNewContext(view,sandbox);
  for (let i=0; i<10 && content.style.display !== ''; i++) {
    await new Promise(resolve=>setImmediate(resolve));
  }
  assert.equal(content.style.display,'','decrypted preview must be revealed');
  assert.deepEqual(positions,['header','preview','proxy','buttons'],
    'proxy checkbox must appear directly below preview and above download buttons');
  assert.equal(download.href,'blob:recovered-file');
  assert.equal(fallbackRoot?.className,'no-preview muted');
  const cross = fallbackChildren.find(child=>child.tag==='img');
  assert.equal(cross?.src,'/static/preview-unavailable.png',
    'browser-decrypted files without preview must show the cross PNG');
  assert.equal(cross?.className,'no-preview-cross');
}
testProxyPositionAfterDecryption().then(()=>{
  console.log('Zero-Knowledge fragment continuity and proxy placement checks passed');
}).catch(err=>{console.error(err);process.exitCode=1;});
