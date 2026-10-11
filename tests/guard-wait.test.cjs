const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const vm = require("node:vm");

const source = fs.readFileSync(path.join(__dirname, "../internal/app/static/guard-wait.js"), "utf8");

function runRefresh(content) {
  const timers = [];
  let reloads = 0;
  const location = {
    hash: "#secret-fragment",
    reload() { reloads++; }
  };
  const document = {
    querySelector(selector) {
      assert.equal(selector, 'meta[http-equiv="refresh"]');
      return content == null ? null : { getAttribute(name) {
        assert.equal(name, "content");
        return content;
      } };
    }
  };

  vm.runInNewContext(source, {
    document,
    window: {location},
    setTimeout(callback, delay) { timers.push({callback, delay}); }
  });

  return { timers, location, getReloads: () => reloads };
}

const waiting = runRefresh("5");
assert.equal(waiting.timers.length, 1);
assert.equal(waiting.timers[0].delay, 5250);
assert.equal(waiting.getReloads(), 0);
waiting.timers[0].callback();
assert.equal(waiting.getReloads(), 1);
assert.equal(waiting.location.hash, "#secret-fragment", "refresh must preserve ZK fragment");

for (const invalid of [null, "", "0", "5;url=https://evil.example/", "3601", "-1", "NaN"]) {
  assert.equal(runRefresh(invalid).timers.length, 0, "do not schedule an unsafe refresh for " + invalid);
}

console.log("OnionGuard refresh fallback, delay bounds and fragment continuity passed");
