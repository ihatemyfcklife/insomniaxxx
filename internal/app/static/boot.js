// Progressive enhancement: local (Zero-Knowledge) encryption when Wasm loads.
// Without JS/Wasm (e.g. Tor Browser Safest) the classic POST form is used.
if (typeof WebAssembly !== "undefined") {
  (async function () {
    try {
      const go = new Go();
      const resp = await fetch("/static/app.wasm");
      if (!resp.ok) {
        return; // Bundle non compilé : repli sur le flux serveur classique.
      }
      const result = await WebAssembly.instantiate(await resp.arrayBuffer(), go.importObject);
      go.run(result.instance);
    } catch (e) {
      return; // En cas d'erreur Wasm, repli silencieux sur le POST HTML.
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

    function setArchMode(mode) {
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
      modeToggle.style.display = "inline-flex";
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
      const qrEndpoint = "/qr?data=" + encodeURIComponent(shareURL);
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
              <img src="${qrEndpoint}" alt="QR code" width="256" height="256">
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

    const RLNC_THRESHOLD = 2 * 1024 * 1024; // 2 Mo

    form.onsubmit = async (e) => {
      e.preventDefault();
      const input = document.getElementById("file");
      const file = input && input.files && input.files[0];
      if (!file) return;

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
        const bytes = new Uint8Array(await file.arrayBuffer());
        const enc = wasmEncrypt(bytes, file.name, file.type || "application/octet-stream");
        if (!enc || enc.error) {
          alert((enc && enc.error) || "Encryption failed.");
          resetBtnState();
          return;
        }

        // Cas 1 : Fichier <= 2 Mo -> upload direct d'un blob monolithique classique
        if (file.size <= RLNC_THRESHOLD) {
          setBtnState("Uploading...");

          const fd = new FormData();
          fd.append("file", new Blob([enc.ciphertext], { type: "application/octet-stream" }), "payload.bin");

          const resp = await fetch("/upload-raw", { method: "POST", body: fd });
          if (!resp.ok) {
            const detail = await resp.text();
            alert("Upload failed: " + (detail || resp.status));
            resetBtnState();
            return;
          }

          const info = await resp.json();
          const shareURL = window.location.origin + "/d/" + info.hash + "#" + enc.key;
          renderReady(file.name, file.size, info.hash, enc.key, shareURL);
          return;
        }

        // Cas 2 : Fichier > 2 Mo -> éparpillement RLNC (K=4, N=6 par défaut)
        setBtnState("Encoding RLNC shards...");

        const k = Math.min(64, Math.max(4, Math.ceil(file.size / (1024 * 1024))));
        const n = Math.max(6, k + Math.floor(k / 2));

        const shards = wasmEncodeRLNC(enc.ciphertext, k, n);
        if (!shards || shards.error) {
          alert("RLNC encoding failed: " + ((shards && shards.error) || "unknown error"));
          resetBtnState();
          return;
        }

        const shardHashes = [];
        for (let i = 0; i < shards.length; i++) {
          setBtnState(`Uploading shard ${i + 1} of ${shards.length}...`);

          const fd = new FormData();
          fd.append("file", new Blob([shards[i]], { type: "application/octet-stream" }), `shard_${i}.bin`);

          const sResp = await fetch("/upload-raw", { method: "POST", body: fd });
          if (!sResp.ok) {
            const detail = await sResp.text();
            alert(`Upload of shard ${i + 1} failed: ` + (detail || sResp.status));
            resetBtnState();
            return;
          }
          const sInfo = await sResp.json();
          shardHashes.push(sInfo.hash);
        }

        setBtnState("Publishing manifest...");

        let relays = [];
        try {
          const rResp = await fetch("/relays?json=1");
          if (rResp.ok) {
            relays = await rResp.json();
          }
        } catch (re) {}
        if (!relays || relays.length === 0) {
          relays = [window.location.origin];
        }

        const manifest = {
          v: 1,
          size: file.size,
          enc_size: enc.ciphertext.length,
          chunk_size: shards[0].length,
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

        const mFd = new FormData();
        mFd.append("file", new Blob([encManifest.ciphertext], { type: "application/octet-stream" }), "manifest.bin");

        const mResp = await fetch("/upload-raw", { method: "POST", body: mFd });
        if (!mResp.ok) {
          const detail = await mResp.text();
          alert("Manifest upload failed: " + (detail || mResp.status));
          resetBtnState();
          return;
        }

        const mInfo = await mResp.json();
        const shareURL = window.location.origin + "/d/" + mInfo.hash + "#" + enc.key;
        renderReady(file.name, file.size, mInfo.hash, enc.key, shareURL);

      } catch (err) {
        alert("Upload error: " + err.message);
        resetBtnState();
      }
    };
  })();
}