// WASM upload orchestration. File.slice() bounds plaintext read buffers to
// 1 MiB; ciphertext is emitted as authenticated packets and stored in a Blob.
// The existing RLNC encoder still materializes its ciphertext/shards in memory.
(function (root) {
  "use strict";
  const CHUNK_BYTES = 1 << 20;
  const MAX_SHARDS = 96;

  function streamSupported() {
    return typeof root.wasmEncryptStreamStart === "function" &&
      typeof root.wasmEncryptStreamPush === "function" &&
      typeof root.wasmEncryptStreamFinish === "function" &&
      typeof root.wasmEncryptStreamAbort === "function";
  }

  async function encryptFile(file, progress) {
    if (!streamSupported()) return null;
    if (!file || !Number.isSafeInteger(file.size) || file.size < 0 ||
        file.size > 64 * CHUNK_BYTES || typeof file.slice !== "function") {
      throw new Error("invalid file for chunk encryption");
    }
    const mime = file.type || "application/octet-stream";
    const start = root.wasmEncryptStreamStart(file.name, mime, file.size);
    if (!start || start.error || !Number.isSafeInteger(start.id) ||
        typeof start.key !== "string" || !start.header) {
      throw new Error((start && start.error) || "streaming encryption could not begin");
    }
    let finished = false;
    try {
      const parts = [start.header];
      for (let offset = 0; offset < file.size; offset += CHUNK_BYTES) {
        // Reading a slice, not file.arrayBuffer(), prevents another complete
        // plaintext copy alongside the Go WASM heap and encrypted output.
        const chunk = new Uint8Array(await file.slice(offset, offset + CHUNK_BYTES).arrayBuffer());
        const packets = root.wasmEncryptStreamPush(start.id, chunk);
        if (!Array.isArray(packets) || packets.error) {
          throw new Error((packets && packets.error) || "streaming encryption failed");
        }
        parts.push(...packets);
        if (typeof progress === "function") progress(Math.min(file.size, offset + chunk.length), file.size);
      }
      const result = root.wasmEncryptStreamFinish(start.id);
      if (!result || result.error || !Array.isArray(result.chunks) ||
          !/^[a-f0-9]{64}$/.test(result.hash)) {
        throw new Error((result && result.error) || "streaming encryption could not finish");
      }
      finished = true;
      parts.push(...result.chunks);
      return {
        ciphertext: new Blob(parts, {type: "application/octet-stream"}),
        key: start.key,
        hash: result.hash
      };
    } finally {
      if (!finished) root.wasmEncryptStreamAbort(start.id);
    }
  }

  // Cap concurrent network transfers so hundreds of RLNC shards cannot
  // overwhelm a browser's sockets, the relay network or the explicitly selected app proxy.
  // Keep result ordering independent of request completion ordering.
  async function uploadShards(shards, uploadOne, progress, concurrency = 3) {
    if (!Array.isArray(shards) || shards.length < 1 || shards.length > MAX_SHARDS ||
        typeof uploadOne !== "function") throw new Error("invalid shard upload plan");
    const limit = Math.max(1, Math.min(4, Math.floor(concurrency) || 3, shards.length));
    const hashes = new Array(shards.length);
    const BATCH_SIZE = 8;
    let completed = 0;

    // Hash and sign just eight shards locally, then run the network workers.
    // Never pre-sign a full 96-shard transfer: Blossom
    // credentials expire after five minutes and upload links can be slow.
    for (let start = 0; start < shards.length; start += BATCH_SIZE) {
      const end = Math.min(shards.length, start + BATCH_SIZE);
      const group = [];
      const seen = new Set();
      for (let i = start; i < end; i++) {
        if (typeof root.wasmSHA256 !== "function") throw new Error("WASM SHA-256 unavailable");
        const hash = root.wasmSHA256(shards[i]);
        if (!/^[a-f0-9]{64}$/.test(hash) || seen.has(hash))
          throw new Error("invalid or repeated RLNC shard hash");
        seen.add(hash);
        hashes[i] = hash;
        group.push({
          hash,
          size: shards[i] instanceof Blob ? shards[i].size : shards[i].byteLength
        });
      }
      const direct = root.InsomniaDirect;
      try {
        if (direct && typeof direct.prepareUploadBatch === "function") {
          // Credentials are generated locally; proxy mode needs none here.
          await direct.prepareUploadBatch(group);
        }
        let cursor = start;
        let failure = null;
        async function worker() {
          while (!failure && cursor < end) {
            const index = cursor++;
            try {
              const result = await uploadOne(shards[index], hashes[index], index);
              if (!result || result.hash !== hashes[index])
                throw new Error("relay shard receipt mismatch");
              completed++;
              if (typeof progress === "function") progress(completed, shards.length);
            } catch (err) {
              if (!failure) failure = err;
            }
          }
        }
        await Promise.all(Array.from({length: Math.min(limit, end - start)}, () => worker()));
        if (failure) throw failure;
      } finally {
        // Credentials are bearer tokens: never keep them beyond this group.
        if (direct && typeof direct.clearPreparedGrants === "function")
          direct.clearPreparedGrants();
      }
    }
    return hashes;
  }

  root.InsomniaStream = {encryptFile, uploadShards};
})(window);
