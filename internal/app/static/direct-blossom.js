// Browser WASM transport: go directly to configured Blossom relays when CORS
// permits. The encrypted app proxy is used only when explicitly selected.
// NEVER transmit share keys to relay URLs or JSON control endpoints.
(function (root) {
  "use strict";
  const HEX = /^[a-f0-9]{64}$/;
  const MAX_BLOB = 68 * 1024 * 1024;
  const DIRECT_TIMEOUT_MS = 45000;
  let relayList = null;
  let proxySelection = null;
  const relayHealth = new Map();
  // Ephemeral, bounded grants for the current eight-shard upload group only.
  const preparedGrants = new Map();
  const MAX_AUTH_BATCH = 8;

  function sanitizeTargets(raw, known) {
    const targets = [];
    const seen = new Set();
    if (!Array.isArray(raw)) return targets;
    for (const target of raw.slice(0, 3)) {
      const url = target && allowedRelay(target.url);
      if (!url || !known.includes(url) ||
          typeof target.authorization !== "string" ||
          !/^Nostr [A-Za-z0-9+/=]{50,4096}$/.test(target.authorization) ||
          seen.has(new URL(url).hostname)) continue;
      seen.add(new URL(url).hostname);
      targets.push({url, authorization: target.authorization});
    }
    return targets;
  }

  async function prepareUploadBatch(blobs) {
    preparedGrants.clear();
    if (proxyOnly()) return false;
    if (!Array.isArray(blobs) || blobs.length < 1 || blobs.length > MAX_AUTH_BATCH)
      throw new Error("invalid upload authorization batch");
    const seen = new Set();
    for (const blob of blobs) {
      if (!blob || !validHash(blob.hash) || seen.has(blob.hash) ||
          !Number.isSafeInteger(blob.size) || blob.size < 16 || blob.size > MAX_BLOB) {
        throw new Error("invalid encrypted authorization inventory");
      }
      seen.add(blob.hash);
    }
    try {
      const known = await configuredRelays();
      if (known.length < 2) return false;
      for (const blob of blobs) {
        preparedGrants.set(blob.hash, localTargets(known, blob.hash));
      }
      return true;
    } catch (_) {
      preparedGrants.clear();
      return false;
    }
  }

  function localTargets(known, hash) {
    if (typeof root.wasmUploadAuthorization !== "function")
      throw new Error("Local relay signing unavailable; reload the page.");
    const targets = known.map(url => ({
      url, authorization: root.wasmUploadAuthorization(url, hash)
    }));
    const valid = sanitizeTargets(targets, known);
    if (valid.length < 2) throw new Error("Local relay signing failed.");
    return valid;
  }

  function clearPreparedGrants() { preparedGrants.clear(); }

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
    if (proxySelection !== null) return proxySelection;
    try {
      const saved = root.sessionStorage.getItem("insomnia:proxy-only");
      if (saved !== null) return saved === "1";
    } catch (_) {}
    return false; // Clearnet uses direct transfers unless proxy is selected.
  }

  const preference = document.getElementById("proxyOnly");
  if (preference) {
    preference.checked = proxyOnly();
    preference.addEventListener("change", () => {
      proxySelection = preference.checked;
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
    if (!proxyOnly()) throw new Error("Direct download failed. Select the encrypted proxy option to retry.");
    // Proxying is an explicit transport choice; keys still stay local.
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

  async function uploadEncrypted(data, hash, { shardIndex = null } = {}) {
    const dataSize = data instanceof Blob ? data.size : data && data.byteLength;
    if (!validHash(hash) || !Number.isSafeInteger(dataSize) ||
        dataSize < 16 || dataSize > MAX_BLOB ||
        !await localHashMatches(data, hash)) {
      throw new Error("encrypted upload SHA-256 or size is invalid");
    }
    if (proxyOnly()) return proxyUpload(data, hash);
    // Consume a preauthorized grant once. It was bound to this exact hash,
    // and is never persisted or reused across separate groups.
    let targets = preparedGrants.get(hash) || [];
    preparedGrants.delete(hash);
    if (!targets.length) {
      try {
        const known = await configuredRelays();
        if (known.length >= 2) {
          targets = localTargets(known, hash);
        }
      } catch (_) { /* unsupported batch or relay: proxy fallback */ }
    }

    if (targets.length >= 2) {
      // A shard is stored on two independent hosts, not sent to every relay.
      // Rotate the pair across A+B, B+C, C+A for three hosts. On errors,
      // use the remaining configured host before the original app proxy.
      const available = targets.filter(target =>
        (relayHealth.get(target.url)?.cooldownUntil || 0) <= Date.now());
      if (available.length >= 2) targets = available;
      const offset = Number.isSafeInteger(shardIndex) && shardIndex >= 0
        ? shardIndex % targets.length : 0;
      const ordered = targets.map((_, i) => targets[(offset + i) % targets.length]);
      const uploadOneTarget = async target => {
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
      };
      const firstPair = await Promise.all(ordered.slice(0, 2).map(uploadOneTarget));
      let successes = firstPair.filter(Boolean).length;
      for (const spare of ordered.slice(2)) {
        if (successes >= 2) break;
        if (await uploadOneTarget(spare)) successes++;
      }
      if (successes >= 2) return { hash, direct: true };
    }
    // A partial one-relay success is not enough: the proxy must still obtain
    // two separate valid receipts before a share link may be published.
    throw new Error("Direct upload failed. Select the encrypted proxy option to retry through insomniaXXX.");
  }

  root.InsomniaDirect = {
    uploadEncrypted, downloadEncrypted, prepareUploadBatch, clearPreparedGrants
  };
})(window);
