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
console.log('OnionGuard Zero-Knowledge fragment continuity checks passed');
