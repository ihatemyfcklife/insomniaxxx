// Browser WASM transport: go directly to configured Blossom relays when CORS
// permits, and fall back to insomniaXXX's existing encrypted proxy otherwise.
// NEVER transmit share keys to relay URLs or JSON control endpoints.
(function (root) {
  "use strict";
  const HEX = /^[a-f0-9]{64}$/;
  const MAX_BLOB = 68 * 1024 * 1024;
  const DIRECT_TIMEOUT_MS = 45000;
  let relayList = null;
  const relayHealth = new Map();

  // Prefer relays that recently returned valid hashes/receipts. Passive
  // health tracking only: never probe another origin without user consent.
  function observeRelay(url, success) {
    const item = relayHealth.get(url) || { failures: 0, cooldownUntil: 0 };
    if (success) {
      item.failures = 0;
      item.cooldownUntil = 0;
    } else {
      item.failures = Math.min(8, item.failures + 1);
      item.cooldownUntil = Date.now() + Math.min(120000, 2500 * (2 ** item.failures));
    }
    relayHealth.set(url, item);
  }

  function rankedRelays(urls) {
    return [...urls].sort((a, b) =>
      (relayHealth.get(a)?.cooldownUntil || 0) - (relayHealth.get(b)?.cooldownUntil || 0));
  }

  // A direct relay learns the browser's public network/Tor exit address.
  // Anyone can choose app-proxied transfers for additional relay privacy.
  function proxyOnly() {
    try {
      const saved = root.sessionStorage.getItem("insomnia:proxy-only");
      if (saved !== null) return saved === "1";
    } catch (_) {}
    // A visitor opening a shared link must be able to decide BEFORE their
    // browser reveals an IP/Tor exit to storage relays. Start viewers in
    // proxy mode until they explicitly uncheck the box (which reloads).
    return !!(root.location && root.location.pathname &&
      root.location.pathname.startsWith("/d/"));
  }

  const preference = document.getElementById("proxyOnly");
  if (preference) {
    preference.checked = proxyOnly();
    preference.addEventListener("change", () => {
      try {
        root.sessionStorage.setItem("insomnia:proxy-only", preference.checked ? "1" : "0");
      } catch (_) {}
      // A viewer may have already begun fetching; reload for an immediate
      // transport switch without losing the fragment-based decryption key.
      if (root.location.pathname.startsWith("/d/")) root.location.reload();
    });
  }

  function allowedRelay(raw) {
    if (typeof raw !== "string" || raw.length > 300) return null;
    try {
      const parsed = new URL(raw);
      if (parsed.protocol !== "https:" ||
          parsed.username || parsed.password || parsed.search || parsed.hash ||
          (parsed.pathname !== "/" && parsed.pathname !== "")) return null;
      return parsed.origin;
    } catch (_) { return null; }
  }

  async function configuredRelays() {
    if (relayList) return relayList;
    const response = await fetch("/direct-relays", {
      credentials: "same-origin", cache: "no-store", redirect: "error"
    });
    if (!response.ok) throw new Error("direct relay discovery unavailable");
    const info = await response.json();
    if (!info || !Array.isArray(info.relays)) throw new Error("invalid direct relay list");
    const known = new Set();
    const relays = [];
    for (const raw of info.relays.slice(0, 3)) {
      const url = allowedRelay(raw);
      if (url && !known.has(new URL(url).hostname)) {
        known.add(new URL(url).hostname);
        relays.push(url);
      }
    }
    if (!relays.length) throw new Error("no HTTPS relays available for direct transfer");
    relayList = relays;
    return relays;
  }

  function validHash(hash) {
    return typeof hash === "string" && HEX.test(hash);
  }

  async function localHashMatches(data, hash) {
    if (data instanceof Blob) {
      if (typeof root.wasmSHA256Begin !== "function" ||
          typeof root.wasmSHA256Push !== "function" ||
          typeof root.wasmSHA256End !== "function") return false;
      const id = root.wasmSHA256Begin();
      if (!Number.isInteger(id)) return false;
      try {
        for (let offset = 0; offset < data.size; offset += 1 << 20) {
          const next = new Uint8Array(await data.slice(offset, offset + (1 << 20)).arrayBuffer());
          if (root.wasmSHA256Push(id, next) !== true) return false;
        }
        return root.wasmSHA256End(id) === hash;
      } finally {
        if (typeof root.wasmSHA256Abort === "function") root.wasmSHA256Abort(id);
      }
    }
    return typeof root.wasmSHA256 === "function" &&
      root.wasmSHA256(data) === hash;
  }

  async function boundedRead(response, limit) {
    const length = response.headers && Number(response.headers.get("Content-Length"));
    if (Number.isFinite(length) && length > limit) throw new Error("oversized relay response");
    if (response.body && typeof response.body.getReader === "function") {
      const reader = response.body.getReader();
      const chunks = [];
      let total = 0;
      try {
        while (true) {
          const { done, value } = await reader.read();
          if (done) break;
          if (!value) continue;
          total += value.byteLength;
          if (total > limit) throw new Error("oversized relay response");
          chunks.push(value);
        }
      } finally {
        if (total > limit) await reader.cancel().catch(() => {});
        reader.releaseLock();
      }
      const result = new Uint8Array(total);
      let offset = 0;
      for (const chunk of chunks) {
        result.set(chunk, offset);
        offset += chunk.byteLength;
      }
      return result;
    }
    // Browser implementations without streamed Fetch bodies.
    const bytes = new Uint8Array(await response.arrayBuffer());
    if (bytes.length > limit) throw new Error("oversized relay response");
    return bytes;
  }

  async function timedFetch(url, options, externalSignal, consumeResponse) {
    const controller = new AbortController();
    const relayAbort = () => controller.abort();
    if (externalSignal) {
      if (externalSignal.aborted) throw new Error("transfer cancelled");
      externalSignal.addEventListener("abort", relayAbort, { once: true });
    }
    const timer = setTimeout(() => controller.abort(), DIRECT_TIMEOUT_MS);
    try {
      const response = await fetch(url, { ...options, signal: controller.signal });
      return await consumeResponse(response);
    } finally {
      clearTimeout(timer);
      if (externalSignal) externalSignal.removeEventListener("abort", relayAbort);
    }
  }

  async function downloadEncrypted(hash, { maxBytes = MAX_BLOB, signal } = {}) {
    if (!validHash(hash) || !Number.isSafeInteger(maxBytes) ||
        maxBytes <= 0 || maxBytes > MAX_BLOB) throw new Error("invalid encrypted download");
    let relays = [];
    if (!proxyOnly()) {
      try { relays = await configuredRelays(); } catch (_) { /* proxy fallback */ }
    }
    for (const relay of rankedRelays(relays)) {
      if (signal && signal.aborted) throw new Error("transfer cancelled");
      try {
        const data = await timedFetch(relay + "/" + hash, {
          method: "GET", mode: "cors", credentials: "omit",
          referrerPolicy: "no-referrer", redirect: "error", cache: "no-store"
        }, signal, response => response.ok ? boundedRead(response, maxBytes) : null);
        if (data && await localHashMatches(data, hash)) {
          observeRelay(relay, true);
          return data;
        }
      } catch (_) {
        if (signal && signal.aborted) throw new Error("transfer cancelled");
        // Unavailable relay, invalid bytes, CORS or timeout: try next target.
      }
      observeRelay(relay, false);
    }
    if (signal && signal.aborted) throw new Error("transfer cancelled");
    // Trusted app proxy remains the interoperable fallback for non-CORS relays.
    const proxy = await fetch("/d/" + hash + "?cipher=1", {
      signal, credentials: "same-origin", cache: "no-store",
      referrerPolicy: "no-referrer", redirect: "error"
    });
    if (!proxy.ok) throw new Error("encrypted blob unavailable (HTTP " + proxy.status + ")");
    const data = await boundedRead(proxy, maxBytes);
    if (!await localHashMatches(data, hash)) throw new Error("encrypted blob integrity check failed");
    return data;
  }

  async function proxyUpload(data, hash) {
    const form = new FormData();
    form.append("file", new Blob([data], { type: "application/octet-stream" }), "payload.bin");
    let response;
    for (let attempt = 0; attempt < 3; attempt++) {
      response = await fetch("/upload-raw", {
        method: "POST", body: form, credentials: "same-origin",
        referrerPolicy: "no-referrer", redirect: "error"
      });
      if (response.ok || ![429, 502, 503, 504].includes(response.status) || attempt === 2) break;
      await new Promise(resolve => setTimeout(resolve, 500 * (attempt + 1)));
    }
    if (!response.ok) throw new Error("encrypted upload unavailable (HTTP " + response.status + ")");
    const receipt = await response.json();
    if (!receipt || receipt.hash !== hash) throw new Error("proxy returned an invalid blob hash");
    return { hash, direct: false };
  }

  async function uploadEncrypted(data, hash) {
    const dataSize = data instanceof Blob ? data.size : data && data.byteLength;
    if (!validHash(hash) || !Number.isSafeInteger(dataSize) ||
        dataSize < 16 || dataSize > MAX_BLOB ||
        !await localHashMatches(data, hash)) {
      throw new Error("encrypted upload SHA-256 or size is invalid");
    }
    if (proxyOnly()) return proxyUpload(data, hash);
    let targets = [];
    try {
      const known = await configuredRelays();
      if (known.length >= 2) {
        const response = await fetch("/direct-upload-auth", {
          method: "POST", credentials: "same-origin", redirect: "error",
          referrerPolicy: "no-referrer", cache: "no-store",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ hash, size: dataSize })
        });
        if (response.ok) {
          const grant = await response.json();
          if (grant && Array.isArray(grant.targets)) {
            const seen = new Set();
            for (const target of grant.targets.slice(0, 3)) {
              const url = target && allowedRelay(target.url);
              if (!url || !known.includes(url) ||
                  !target.authorization || !/^Nostr [A-Za-z0-9+/=]{50,4096}$/.test(target.authorization) ||
                  seen.has(new URL(url).hostname)) continue;
              seen.add(new URL(url).hostname);
              targets.push({ url, authorization: target.authorization });
            }
          }
        }
      }
    } catch (_) { /* failed authorization or offline app: proxy fallback */ }

    if (targets.length >= 2) {
      // Avoid known CORS-incompatible/unavailable relays while we still have
      // two independent targets; failures expire after a bounded cooldown.
      const available = targets.filter(target =>
        (relayHealth.get(target.url)?.cooldownUntil || 0) <= Date.now());
      if (available.length >= 2) targets = available;
      // At most three relay requests per ciphertext blob, without involving
      // insomniaXXX in the actual upload body. Never pass cookies/keys.
      const results = await Promise.all(targets.map(async target => {
        try {
          const receiptBytes = await timedFetch(target.url + "/upload", {
            method: "PUT", mode: "cors", credentials: "omit",
            referrerPolicy: "no-referrer", redirect: "error",
            headers: {
              "Authorization": target.authorization,
              "X-SHA-256": hash,
              "Content-Type": "application/octet-stream"
            }, body: data
          }, null, response => response.ok ? boundedRead(response, 4096) : null);
          if (!receiptBytes) {
            observeRelay(target.url, false);
            return false;
          }
          const receipt = JSON.parse(new TextDecoder().decode(receiptBytes));
          const valid = !!receipt && typeof receipt.sha256 === "string" &&
            receipt.sha256.toLowerCase() === hash &&
            (receipt.size === undefined || receipt.size === dataSize);
          observeRelay(target.url, valid);
          return valid;
        } catch (_) {
          observeRelay(target.url, false);
          return false;
        }
      }));
      if (results.filter(Boolean).length >= 2) return { hash, direct: true };
    }
    // A partial one-relay success is not enough: the proxy must still obtain
    // two separate valid receipts before a share link may be published.
    return proxyUpload(data, hash);
  }

  root.InsomniaDirect = { uploadEncrypted, downloadEncrypted };
})(window);
