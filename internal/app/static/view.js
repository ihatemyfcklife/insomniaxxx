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

  const keyText = window.location.hash.charAt(0) === "#" ? window.location.hash.slice(1) : "";
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
    const resp = await fetch(cipherURL);
    if (!resp.ok) {
      setStatus("Ciphertext unavailable (HTTP " + resp.status + ").", true);
      return;
    }
    cipher = new Uint8Array(await resp.arrayBuffer());
  } catch (e) {
    setStatus("Ciphertext download failed.", true);
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
      if (type.indexOf("image/") === 0) {
        el = document.createElement("img");
        el.className = "preview-media";
        el.src = url;
        el.alt = name;
      } else if (type.indexOf("video/") === 0) {
        el = document.createElement("video");
        el.className = "preview-media";
        el.controls = true;
        el.preload = "metadata";
        el.src = url;
      } else if (type.indexOf("audio/") === 0) {
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
        box.innerHTML = `<div class="no-preview muted"><p>No preview available for ${type} — use Download.</p></div>`;
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

  const abortController = new AbortController();
  const acceptedShards = [];
  const candidateRelays = (manifest.relays && manifest.relays.length > 0) ? manifest.relays : [window.location.origin];

  async function fetchShard(hash) {
    const urlsToTry = [];
    urlsToTry.push(`/d/${hash}?cipher=1`);
    for (const r of candidateRelays) {
      const cleanRelay = r.replace(/\/$/, "");
      urlsToTry.push(`${cleanRelay}/${hash}`);
    }

    for (const url of urlsToTry) {
      if (abortController.signal.aborted) return null;
      try {
        const resp = await fetch(url, { signal: abortController.signal });
        if (resp.ok) {
          const buf = new Uint8Array(await resp.arrayBuffer());
          // Contrôle de forme : en-tête ABSH
          if (buf.length >= 40 && buf[0] === 0x41 && buf[1] === 0x42 && buf[2] === 0x53 && buf[3] === 0x48) {
            return buf;
          }
        }
      } catch (err) {
        if (abortController.signal.aborted) return null;
      }
    }
    return null;
  }

  const shardPromises = manifest.shards.map((hash) =>
    fetchShard(hash).then((shardBytes) => {
      if (!shardBytes || abortController.signal.aborted) return;
      if (typeof wasmIsInnovative === "function") {
        if (!wasmIsInnovative(acceptedShards, shardBytes)) {
          return;
        }
      }
      acceptedShards.push(shardBytes);
      setStatus(`Fetching shards from distributed relays (${acceptedShards.length} / ${manifest.k})…`);
      if (acceptedShards.length >= manifest.k) {
        abortController.abort();
      }
    })
  );

  await Promise.allSettled(shardPromises);

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