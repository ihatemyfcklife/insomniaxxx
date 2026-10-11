// Zero-Knowledge view: read the key from the URL fragment, fetch the raw
// ciphertext (or RLNC manifest), decrypt and decode locally in Wasm, then reveal the preview.
(async function () {
  const script = document.currentScript;
  const cipherURL = script ? script.getAttribute("data-cipher") : "";
  const loader = document.getElementById("zkLoader");
  const content = document.getElementById("zkContent");
  const spinner = document.getElementById("zkSpinner");
  const visual = document.getElementById("zkVisual");
  const statusTitle = document.getElementById("zkStatusTitle");
  const status = document.getElementById("zkStatus");
  const box = document.getElementById("previewBox");
  const dl = document.getElementById("zkDownload");
  const proxyOptions = document.getElementById("zkProxyOptions");

  function setStatus(msg, isError) {
    if (status) {
      status.textContent = msg;
    }
    if (isError) {
      if (spinner) spinner.style.display = "none";
      if (visual) visual.style.display = "none";
      if (statusTitle) {
        statusTitle.textContent = "Error";
        statusTitle.style.color = "var(--pico-del-color, #ff6b6b)";
      }
      // The preview stays hidden on failure. Make the existing proxy toggle
      // reachable under the error message so a failed direct fetch can be retried.
      if (loader && proxyOptions) loader.appendChild(proxyOptions);
    }
  }

  function formatSize(n) {
    if (n < 1024) {
      return n + " B";
    }
    const units = ["KB", "MB", "GB"];
    let value = n;
    let i = -1;
    do {
      value /= 1024;
      i++;
    } while (value >= 1024 && i < units.length - 1);
    return value.toFixed(1) + " " + units[i];
  }

  // OnionGuard may have shown a no-JS CAPTCHA using a 200 HTML success
  // page followed by a refresh. Fragments are never sent in form submissions.
  // Recover only a short-lived, same-tab secret captured on the gate page.
  const fragmentSlot = "insomnia:pending-zk:" + window.location.pathname;
  let keyText = window.location.hash.charAt(0) === "#" ? window.location.hash.slice(1) : "";
  try {
    const pendingRaw = sessionStorage.getItem(fragmentSlot);
    if (keyText) {
      sessionStorage.removeItem(fragmentSlot);
    } else if (pendingRaw && /^\/d\/[a-f0-9]{64}$/.test(window.location.pathname)) {
      sessionStorage.removeItem(fragmentSlot);
      const pending = JSON.parse(pendingRaw);
      if (pending.expires > Date.now() && /^[A-Za-z0-9_-]{43}$/.test(pending.key)) {
        keyText = pending.key;
        window.history.replaceState(null, "", window.location.pathname + window.location.search + "#" + keyText);
      }
    }
  } catch (_) { /* private browsing can disable sessionStorage */ }
  if (!keyText) {
    setStatus("Missing decryption key in the URL fragment (#…).", true);
    return;
  }
  if (typeof WebAssembly === "undefined") {
    setStatus("WebAssembly is unavailable in this browser mode.", true);
    return;
  }

  try {
    setStatus("Initializing WebAssembly cryptographic engine…");
    const go = new Go();
    const wasmResp = await fetch("/static/app.wasm");
    if (!wasmResp.ok) {
      setStatus("Wasm bundle missing (run make wasm).", true);
      return;
    }
    const result = await WebAssembly.instantiate(await wasmResp.arrayBuffer(), go.importObject);
    go.run(result.instance);
  } catch (e) {
    setStatus("Wasm initialization failed: " + e.message, true);
    return;
  }

  let cipher;
  try {
    setStatus("Downloading encrypted payload…");
    const match = /^\/d\/([a-f0-9]{64})\?cipher=1$/.exec(cipherURL);
    if (!match) throw new Error("invalid ciphertext path");
    if (window.InsomniaDirect) {
      cipher = await window.InsomniaDirect.downloadEncrypted(match[1]);
    } else {
      throw new Error("Transfer helper unavailable; reload the page.");
    }
  } catch (e) {
    setStatus(e.message || "Ciphertext download failed.", true);
    return;
  }

  setStatus("Decrypting payload…");
  const dec = wasmDecrypt(cipher, keyText);
  if (!dec || dec.error) {
    setStatus((dec && dec.error) || "Decryption failed.", true);
    return;
  }

  // Vérifier si le blob déchiffré est un manifeste RLNC
  let manifest = null;
  if (dec.type === "application/x-anonblossom-manifest+json") {
    try {
      manifest = JSON.parse(new TextDecoder().decode(dec.data));
    } catch (e) {}
  } else {
    try {
      const parsed = JSON.parse(new TextDecoder().decode(dec.data));
      if (parsed && parsed.v === 1 && Array.isArray(parsed.shards) && parsed.k > 0) {
        manifest = parsed;
      }
    } catch (e) {}
  }

  function displayMedia(dataBytes, name, type, size) {
    // A failed attempt may have moved the proxy checkbox into the loading
    // panel so it stays accessible. Restore its position once preview succeeds.
    // Also enforce the layout if an older rendered page placed it above the file.
    if (box && proxyOptions) box.insertAdjacentElement("afterend", proxyOptions);

    // Révéler le contenu de preview et masquer l'animation de chargement
    if (loader) loader.style.display = "none";
    if (content) content.style.display = "";

    const blob = new Blob([dataBytes], { type: type || "application/octet-stream" });
    const url = URL.createObjectURL(blob);

    const nameEl = document.getElementById("previewName");
    if (nameEl) nameEl.textContent = name;
    const sizeEl = document.getElementById("previewSize");
    if (sizeEl) sizeEl.textContent = formatSize(Number(size));
    const typeEl = document.getElementById("previewType");
    if (typeEl) typeEl.textContent = type;

    document.title = name + " · " + (document.title.split(" · ")[1] || "anonblossom");

    if (box) {
      let el = null;
      if (["image/png", "image/jpeg", "image/gif", "image/webp"].includes(type)) {
        el = document.createElement("img");
        el.className = "preview-media";
        el.src = url;
        el.alt = name;
      } else if (["video/mp4", "video/webm", "video/ogg"].includes(type)) {
        el = document.createElement("video");
        el.className = "preview-media";
        el.controls = true;
        el.preload = "metadata";
        el.src = url;
      } else if (["audio/mpeg", "audio/ogg", "audio/wav", "audio/webm"].includes(type)) {
        el = document.createElement("audio");
        el.className = "preview-audio";
        el.controls = true;
        el.preload = "metadata";
        el.src = url;
      }
      if (el) {
        box.innerHTML = "";
        box.appendChild(el);
      } else {
        const message = document.createElement("p");
        message.textContent = "No preview available for " + type + " — use Download.";
        const fallback = document.createElement("div");
        fallback.className = "no-preview muted";
        fallback.appendChild(message);
        box.replaceChildren(fallback);
      }
    }

    if (dl) {
      dl.href = url;
      dl.setAttribute("download", name || "download");
      dl.innerHTML = `<span class="btn-text">Download (${formatSize(Number(size))})</span><span class="btn-loading"><span class="btn-spinner" aria-hidden="true"></span>Downloading…</span>`;
      dl.style.display = "";
    }
  }

  // Cas 1 : Blob monolithique direct
  if (!manifest) {
    displayMedia(dec.data, dec.name, dec.type, dec.size);
    return;
  }

  // Cas 2 : Manifeste RLNC -> téléchargement parallèle des shards sans afficher la preview
  setStatus(`Fetching shards from distributed relays (0 / ${manifest.k})…`);

  // A manifest is supplied by a third party; validate it before allocating shards.
  if (!Number.isSafeInteger(manifest.k) || manifest.k < 1 || manifest.k > 64 ||
      !Number.isSafeInteger(manifest.n) || manifest.n < manifest.k || manifest.n > 96 ||
      !Array.isArray(manifest.shards) || manifest.shards.length !== manifest.n ||
      !Number.isSafeInteger(manifest.enc_size) || manifest.enc_size <= 0 || manifest.enc_size > 68 * 1024 * 1024 ||
      !Number.isSafeInteger(manifest.chunk_size) ||
      manifest.chunk_size !== Math.ceil(manifest.enc_size / manifest.k) ||
      manifest.shards.some(h => typeof h !== "string" || !/^[a-f0-9]{64}$/.test(h))) {
    setStatus("Invalid RLNC manifest.", true);
    return;
  }

  // Prefer verified direct CORS downloads to the app's configured
  // relays. The same-origin proxy remains the integrity-checked fallback.
  const abortController = new AbortController();
  const acceptedShards = [];
  let nextShard = 0;

  async function fetchShard(hash) {
    for (let attempt = 0; attempt < 3 && !abortController.signal.aborted; attempt++) {
      try {
        let buf;
        if (window.InsomniaDirect) {
          buf = await window.InsomniaDirect.downloadEncrypted(hash, {
            maxBytes: manifest.chunk_size + 40, signal: abortController.signal
          });
        } else {
          throw new Error("Transfer helper unavailable; reload the page.");
        }
        if (buf.length !== manifest.chunk_size + 40 ||
            buf[0] !== 0x41 || buf[1] !== 0x42 || buf[2] !== 0x53 || buf[3] !== 0x48 ||
            typeof wasmSHA256 !== "function" || wasmSHA256(buf) !== hash) {
          continue;
        }
        return buf;
      } catch (err) {
        if (abortController.signal.aborted) return null;
      }
    }
    return null;
  }

  async function worker() {
    while (!abortController.signal.aborted && nextShard < manifest.shards.length) {
      const index = nextShard++;
      const shardBytes = await fetchShard(manifest.shards[index]);
      if (!shardBytes || abortController.signal.aborted) continue;
      if (typeof wasmIsInnovative !== "function" || !wasmIsInnovative(acceptedShards, shardBytes)) continue;
      acceptedShards.push(shardBytes);
      setStatus(`Fetching shards from distributed relays (${acceptedShards.length} / ${manifest.k})…`);
      if (acceptedShards.length >= manifest.k) abortController.abort();
    }
  }

  await Promise.all([worker(), worker()]);

  if (acceptedShards.length < manifest.k) {
    setStatus(`Failed to retrieve enough innovative shards (${acceptedShards.length} / ${manifest.k}).`, true);
    return;
  }

  setStatus("Reconstructing data via RLNC decoding…");
  const targetEncSize = manifest.enc_size || 0;
  const decodedCipher = wasmDecodeRLNC(acceptedShards, manifest.k, targetEncSize);
  if (!decodedCipher || decodedCipher.error) {
    setStatus("RLNC decoding failed: " + ((decodedCipher && decodedCipher.error) || "unknown"), true);
    return;
  }

  setStatus("Decrypting file in memory…");
  const fileDec = wasmDecrypt(decodedCipher, keyText);
  if (!fileDec || fileDec.error) {
    setStatus("Decryption failed: " + ((fileDec && fileDec.error) || "unknown"), true);
    return;
  }

  // Tout est décodé et déchiffré : révéler la prévisualisation !
  displayMedia(fileDec.data, fileDec.name, fileDec.type, fileDec.size);
})();
