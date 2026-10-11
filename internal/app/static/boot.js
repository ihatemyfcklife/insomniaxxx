// Progressive enhancement: local (Zero-Knowledge) encryption when Wasm loads.
// This script is served only on clearnet. Failure never enables plaintext POST.
const localUploadForm = document.getElementById("uploadForm");
if (localUploadForm) localUploadForm.onsubmit = (event) => event.preventDefault();
if (typeof WebAssembly !== "undefined") {
  (async function () {
    try {
      const go = new Go();
      const resp = await fetch("/static/app.wasm");
      if (!resp.ok) {
        throw new Error("WASM bundle unavailable");
      }
      const result = await WebAssembly.instantiate(await resp.arrayBuffer(), go.importObject);
      go.run(result.instance);
    } catch (e) {
      const status = document.getElementById("wasmStatus");
      if (status) status.textContent = "Local encryption unavailable. Upload is disabled; reload or use the onion service.";
      return;
    }

    const form = document.getElementById("uploadForm");
    if (!form) return;

    const status = document.getElementById("wasmStatus");
    if (status) {
      status.textContent = "Zero-Knowledge mode active (local encryption & RLNC)";
    }

    // Architecture Mode Switcher (auto-switches to Wasm and exposes interactive switcher)
    const archImg = document.getElementById("architectureImg");
    const nojsDetails = document.getElementById("architectureDetailsNoJS");
    const wasmDetails = document.getElementById("architectureDetailsWasm");
    const modeToggle = document.getElementById("modeToggle");
    const btnNoJS = document.getElementById("btnModeNoJS");
    const btnWasm = document.getElementById("btnModeWasm");

    // Selecting No-JS switches to native HTML POST; it must not continue
    // encrypting locally after the user explicitly changes modes.
    let activeMode = "wasm";
    function setArchMode(mode) {
      if (mode !== "wasm") return;
      activeMode = mode;
      if (mode === "wasm") {
        if (archImg) archImg.src = "/static/architecture-wasm.png";
        if (nojsDetails) nojsDetails.style.display = "none";
        if (wasmDetails) wasmDetails.style.display = "";
        if (btnNoJS) {
          btnNoJS.classList.add("outline");
          btnNoJS.setAttribute("aria-pressed", "false");
        }
        if (btnWasm) {
          btnWasm.classList.remove("outline");
          btnWasm.setAttribute("aria-pressed", "true");
        }
      } else {
        if (archImg) archImg.src = "/static/architecture-nojs.png";
        if (nojsDetails) nojsDetails.style.display = "";
        if (wasmDetails) wasmDetails.style.display = "none";
        if (btnNoJS) {
          btnNoJS.classList.remove("outline");
          btnNoJS.setAttribute("aria-pressed", "true");
        }
        if (btnWasm) {
          btnWasm.classList.add("outline");
          btnWasm.setAttribute("aria-pressed", "false");
        }
      }
    }

    if (modeToggle && btnNoJS && btnWasm) {
      modeToggle.style.display = "none";
      btnNoJS.addEventListener("click", () => setArchMode("nojs"));
      btnWasm.addEventListener("click", () => setArchMode("wasm"));
      setArchMode("wasm");
    }

    function escapeHTML(value) {
      return String(value).replace(/[&<>"']/g, (c) =>
        ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[c]
      );
    }

    function formatBytes(n) {
      if (n < 1024) return n + " B";
      const units = ["KiB", "MiB", "GiB"];
      let val = n;
      for (let i = 0; i < units.length; i++) {
        val /= 1024;
        if (val < 1024 || i === units.length - 1) {
          return val.toFixed(1) + " " + units[i];
        }
      }
    }

    function renderReady(name, size, hash, key, shareURL) {
      const safeName = escapeHTML(name);
      // Never send a fragment-bearing URL (including its key) to /qr.
      const png = typeof wasmGenerateQR === "function" ? wasmGenerateQR(shareURL) : null;
      const qrURL = png && !png.error ? URL.createObjectURL(new Blob([png], { type: "image/png" })) : "";
      const qrMarkup = qrURL ? `<img src="${qrURL}" alt="QR code" width="256" height="256">` : "";
      const container = document.querySelector("main.container");
      if (!container) return;

      // Récupération dynamique du HTML du footer existant dans la page initiale
      const footerElement = document.querySelector("footer.site-footer");
      const footerHTML = footerElement ? footerElement.outerHTML : "";

      const logoBox = container.querySelector(".logo-box");
      const readyHTML = `
        <header>
          <h1>Ready</h1>
          <p class="muted">Your encrypted file is ready to share.</p>
        </header>
        <article class="success" role="status">
          <div class="share-box">
            <figure class="share-qr">
              ${qrMarkup}
            </figure>
            <div class="share-details">
              <div class="meta-item">
                <small class="muted">File</small>
                <strong>${safeName}</strong>
              </div>
              <div class="meta-item">
                <small class="muted">Size</small>
                <span>${formatBytes(size)}</span>
              </div>
              <div class="meta-item">
                <small class="muted">SHA-256 (Ciphertext / Manifest)</small>
                <code>${hash}</code>
              </div>
              <div class="meta-item">
                <small class="muted">Encryption Key</small>
                <code>${key}</code>
              </div>
            </div>
          </div>
        </article>
        <label for="share">Share link</label>
        <input id="share" value="${shareURL}" readonly>
        <div class="grid">
          <a role="button" href="${shareURL}" class="btn-action"><span class="btn-text">Download / Preview</span><span class="btn-loading"><span class="btn-spinner" aria-hidden="true"></span>Decrypting &amp; Loading...</span></a>
          <a role="button" class="secondary" href="/">Share another</a>
        </div>
        <div class="share-note">
          <small class="muted">Zero-Knowledge: file encrypted in your browser. The secret key never left your device.</small>
        </div>
        ${footerHTML}
      `;

      if (logoBox) {
        container.prepend(logoBox);
        container.querySelectorAll(":scope > :not(.logo-box)").forEach((el) => el.remove());
        logoBox.insertAdjacentHTML("afterend", readyHTML);
      } else {
        container.innerHTML = readyHTML;
      }

      const shareInput = document.getElementById("share");
      if (shareInput) {
        shareInput.addEventListener("click", function () {
          this.select();
        });
      }
    }

    const submit = document.getElementById("submitBtn");
    if (submit) submit.disabled = false;
    async function uploadViaProxy() {
      throw new Error("Transfer helper unavailable; reload the page.");
    }

     form.onsubmit = async (e) => {
      // Never submit plaintext to the clearnet server.
      e.preventDefault();
      const input = document.getElementById("file");
      const file = input && input.files && input.files[0];
      if (!file) return;

      // Match the configured plaintext limit before buffering or encrypting.
      const maxFileSize = Number(input.dataset.maxUploadBytes);
      if (!Number.isSafeInteger(maxFileSize) || maxFileSize <= 0 ||
          file.size > maxFileSize || file.size > 64 * 1024 * 1024) {
        alert("File exceeds the server upload limit.");
        return;
      }

      const submitBtn = document.getElementById("submitBtn");
      function ensureMascot() {
        if (!submitBtn) return;
        let mascot = submitBtn.querySelector("img.upload-btn-mascot");
        if (!mascot) {
          mascot = document.createElement("img");
          mascot.src = "/static/xxx.png";
          mascot.alt = "";
          mascot.className = "upload-btn-mascot";
          mascot.width = 1024;
          mascot.height = 1072;
          mascot.draggable = false;
          mascot.setAttribute("aria-hidden", "true");
          submitBtn.prepend(mascot);
        }
        return mascot;
      }
      function setBtnState(text) {
        if (!submitBtn) return;
        ensureMascot();
        submitBtn.classList.add("is-loading");
        const bt = submitBtn.querySelector(".btn-text");
        if (bt) { bt.style.visibility = "hidden"; bt.style.opacity = "0"; }
        let loading = submitBtn.querySelector(".btn-loading");
        if (!loading) {
          loading = document.createElement("span");
          loading.className = "btn-loading";
          submitBtn.appendChild(loading);
        }
        loading.style.visibility = "visible";
        loading.style.opacity = "1";
        loading.style.transitionDelay = "0s,0s";
        loading.textContent = "";
        const sp = document.createElement("span");
        sp.className = "btn-spinner";
        sp.setAttribute("aria-hidden", "true");
        loading.appendChild(sp);
        loading.appendChild(document.createTextNode(text));
      }
      function resetBtnState() {
        if (!submitBtn) return;
        ensureMascot();
        submitBtn.classList.remove("is-loading");
        submitBtn.disabled = false;
        const bt = submitBtn.querySelector(".btn-text");
        if (bt) { bt.style.visibility = ""; bt.style.opacity = ""; }
        let loading = submitBtn.querySelector(".btn-loading");
        if (!loading) {
          loading = document.createElement("span");
          loading.className = "btn-loading";
          submitBtn.appendChild(loading);
        }
        loading.style.visibility = "";
        loading.style.opacity = "";
        loading.style.transitionDelay = "";
        loading.innerHTML = '<span class="btn-spinner" aria-hidden="true"></span>Uploading &amp; Encrypting...';
      }

      if (submitBtn) {
        submitBtn.disabled = true;
        setBtnState("Encrypting...");
      }

      try {
        let enc = window.InsomniaStream
          ? await window.InsomniaStream.encryptFile(file, (done, total) => {
              if (total > 0) setBtnState("Encrypting " + Math.round(done * 100 / total) + "%...");
            })
          : null;
        if (!enc) {
          // Compatible with older Wasm bundles without the stream API.
          const bytes = new Uint8Array(await file.arrayBuffer());
          enc = wasmEncrypt(bytes, file.name, file.type || "application/octet-stream");
        }
        if (!enc || enc.error) {
          alert((enc && enc.error) || "Encryption failed.");
          resetBtnState();
          return;
        }

        // ALL files, including zero-byte and tiny files, are sharded.
        // Only a short encrypted manifest is replicated as a full blob.
        setBtnState("Encoding distributed RLNC shards...");

        const k = Math.min(64, Math.max(4, Math.ceil(file.size / (1024 * 1024))));
        const n = Math.max(6, k + Math.floor(k / 2));

        // RLNC currently needs random access to the complete encrypted
        // ciphertext. This stage materializes the encrypted buffer, never
        // a second full plaintext copy; small files follow the same path.
        const cipherBytes = enc.ciphertext instanceof Blob
          ? new Uint8Array(await enc.ciphertext.arrayBuffer())
          : enc.ciphertext;
        const shards = wasmEncodeRLNC(cipherBytes, k, n);
        if (!shards || shards.error) {
          alert("RLNC encoding failed: " + ((shards && shards.error) || "unknown error"));
          resetBtnState();
          return;
        }

        const uploadShard = (bytes, hash, shardIndex) => window.InsomniaDirect
          ? window.InsomniaDirect.uploadEncrypted(bytes, hash, { shardIndex })
          : uploadViaProxy(bytes, hash);
        let shardHashes;
        if (window.InsomniaStream) {
          shardHashes = await window.InsomniaStream.uploadShards(
            shards, uploadShard,
            (completed, total) => setBtnState(`Uploaded ${completed} / ${total} shards...`),
            3
          );
        } else {
          // Legacy fallback for mismatched/cached static frontend assets.
          shardHashes = [];
          for (let i = 0; i < shards.length; i++) {
            setBtnState(`Uploading shard ${i + 1} of ${shards.length}...`);
            const hash = typeof wasmSHA256 === "function"
              ? wasmSHA256(shards[i]) : null;
            const result = await uploadShard(shards[i], hash, i);
            shardHashes.push(result.hash);
          }
        }

        setBtnState("Publishing manifest...");

        // Manifests may list configured relays, but browser downloads trust
        // only the app's configured relay list; never fetch arbitrary URLs
        // embedded by a third party in an encrypted manifest.
        let relays = [];
        try {
          const rResp = await fetch("/direct-relays");
          if (rResp.ok) {
            const info = await rResp.json();
            if (info && Array.isArray(info.relays)) relays = info.relays;
          }
        } catch (_) {}

        const manifest = {
          v: 1,
          size: file.size,
          enc_size: enc.ciphertext instanceof Blob
            ? enc.ciphertext.size : enc.ciphertext.length,
          chunk_size: shards[0].length - 40, // serialized ABSH header is not data
          k: k,
          n: n,
          meta: {
            name: file.name,
            type: file.type || "application/octet-stream",
            size: file.size
          },
          shards: shardHashes,
          relays: relays
        };

        const manifestBytes = new TextEncoder().encode(JSON.stringify(manifest));
        const encManifest = wasmEncryptWithKey(
          manifestBytes,
          "manifest.json",
          "application/x-anonblossom-manifest+json",
          enc.key
        );
        if (!encManifest || encManifest.error) {
          alert("Manifest encryption failed: " + (encManifest && encManifest.error));
          resetBtnState();
          return;
        }

        const manifestHash = typeof wasmSHA256 === "function"
          ? wasmSHA256(encManifest.ciphertext) : encManifest.hash;
        const result = window.InsomniaDirect
          ? await window.InsomniaDirect.uploadEncrypted(encManifest.ciphertext, manifestHash)
          : await uploadViaProxy(encManifest.ciphertext, manifestHash);
        const shareURL = window.location.origin + "/d/" + result.hash + "#" + enc.key;
        renderReady(file.name, file.size, result.hash, enc.key, shareURL);

      } catch (err) {
        alert("Upload error: " + err.message);
        resetBtnState();
      }
    };
  })();
}
 else {
  const status = document.getElementById("wasmStatus");
  if (status) status.textContent = "Local encryption requires WebAssembly. Use the onion service for server processing.";
}
