package app

import (
	"encoding/json"
	"io"
	"net/http"
	"os"

	"anonblossom/internal/cryptbox"
)

// handleUploadRaw stocke un blob déjà chiffré dans le navigateur par le bundle
// Wasm. Le serveur ne voit ni le clair ni la clé de partage (Zero-Knowledge).
func (a *Server) handleUploadRaw(w http.ResponseWriter, r *http.Request) {
	if !a.acquireUpload(w, r) {
		return
	}
	defer a.releaseUpload()

	r.Body = http.MaxBytesReader(w, r.Body, a.cfg.MaxUploadBytes+4<<20)
	if err := r.ParseMultipartForm(16 << 20); err != nil {
		http.Error(w, "payload is too large or malformed", http.StatusBadRequest)
		return
	}
	defer r.MultipartForm.RemoveAll()

	fileHeader, ok := singleFile(r.MultipartForm.File["file"])
	if !ok {
		http.Error(w, "choose exactly one file", http.StatusBadRequest)
		return
	}
	// Le chiffré ne dépasse jamais le plafond de clair plus la marge d'encapsulation.
	if fileHeader.Size < 0 || fileHeader.Size > a.cfg.MaxUploadBytes+1<<20 {
		http.Error(w, "file exceeds the upload limit", http.StatusBadRequest)
		return
	}

	file, err := fileHeader.Open()
	if err != nil {
		http.Error(w, "could not read the upload", http.StatusBadRequest)
		return
	}
	defer file.Close()

	tmp, err := os.CreateTemp("", "anonblossom-raw-*")
	if err != nil {
		http.Error(w, "could not prepare the upload", http.StatusServiceUnavailable)
		return
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	defer tmp.Close()

	if _, err := io.Copy(tmp, file); err != nil {
		http.Error(w, "could not buffer the upload", http.StatusBadRequest)
		return
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		http.Error(w, "could not prepare the upload", http.StatusServiceUnavailable)
		return
	}

	// Contrôle de forme : le flux doit commencer par un en-tête cryptbox valide ou un shard RLNC (ABSH).
	outer := make([]byte, 16)
	if _, err := io.ReadFull(tmp, outer); err != nil {
		http.Error(w, "payload is not an encrypted share", http.StatusBadRequest)
		return
	}
	_, isCrypt := cryptbox.ParseOuterHeader(outer)
	isRLNC := string(outer[:4]) == "ABSH"
	if !isCrypt && !isRLNC {
		http.Error(w, "payload is not an encrypted share or RLNC shard", http.StatusBadRequest)
		return
	}

	blobHash, encryptedSize, err := fingerprint(tmp)
	if err != nil {
		http.Error(w, "could not fingerprint the upload", http.StatusInternalServerError)
		return
	}
	if err := a.storage.Upload(r.Context(), tmp, encryptedSize, blobHash); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(map[string]string{"hash": blobHash})
}
