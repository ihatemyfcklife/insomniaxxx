// Keep Zero-Knowledge fragments in the *browser* across an OnionGuard wait
// room / CAPTCHA post / HTML success refresh. Never include the key in form
// inputs, fetch URLs, redirects, analytics, or requests to the server.
(function () {
  const match = /^\\/d\\/[a-f0-9]{64}$/.exec(window.location.pathname);
  if (!match || !/^[A-Za-z0-9_-]{43}$/.test(window.location.hash.slice(1))) return;
  const slot = "insomnia:pending-zk:" + window.location.pathname;
  try {
    sessionStorage.setItem(slot, JSON.stringify({
      key: window.location.hash.slice(1),
      expires: Date.now() + 5 * 60 * 1000
    }));
  } catch (_) { /* no persistent storage fallback */ }
})();
