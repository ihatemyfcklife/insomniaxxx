package app

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"

	"github.com/ihatemyfcklife/badcrypt"

	"anonblossom/internal/cryptbox"
	"anonblossom/internal/qr"
	"anonblossom/internal/rlncbox"
	"anonblossom/internal/web"
)

func (a *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	if !a.acquireUpload(w, r) {
		return
	}
	defer a.releaseUpload()

	r.Body = http.MaxBytesReader(w, r.Body, a.cfg.MaxUploadBytes+4<<20)
	if err := r.ParseMultipartForm(16 << 20); err != nil {
		a.renderHomeError(w, "upload is too large or malformed")
		return
	}
	defer r.MultipartForm.RemoveAll()

	fileHeader, ok := singleFile(r.MultipartForm.File["file"])
	if !ok {
		a.renderHomeError(w, "choose exactly one file")
		return
	}
	if fileHeader.Size < 0 || fileHeader.Size > a.cfg.MaxUploadBytes {
		a.renderHomeError(w, "file exceeds the upload limit")
		return
	}

	file, err := fileHeader.Open()
	if err != nil {
		a.renderHomeError(w, "could not read the uploaded file")
		return
	}
	defer file.Close()

	meta := cryptbox.FileMeta{
		Name: uploadFilename(fileHeader.Filename),
		Type: contentType(fileHeader.Filename, fileHeader.Header.Get("Content-Type")),
		Size: fileHeader.Size,
	}

	// Pour les gros fichiers (> 2 Mo) : éparpillement RLNC redondant
	if meta.Size > rlncbox.Threshold {
		a.handleUploadRLNC(w, r, file, meta)
		return
	}

	tmp, err := os.CreateTemp("", "anonblossom-upload-*")
	if err != nil {
		a.renderHomeError(w, "could not prepare encrypted upload")
		return
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	defer tmp.Close()

	key, err := cryptbox.EncryptShare(file, tmp, meta, a.cfg.ChunkSize)
	if err != nil {
		a.renderHomeError(w, "encryption failed")
		return
	}
	defer badcrypt.Zeroize(key)

	blobHash, encryptedSize, err := fingerprint(tmp)
	if err != nil {
		a.renderHomeError(w, "could not fingerprint encrypted upload")
		return
	}

	if err := a.storage.Upload(r.Context(), tmp, encryptedSize, blobHash); err != nil {
		a.renderHomeError(w, err.Error())
		return
	}

	keyText := cryptbox.ShareKeyText(key)
	shareURL := a.publicBaseURL(r) + "/d/" + blobHash + "/" + keyText

	w.Header().Set("Cache-Control", "no-store")
	_ = a.pages.Done(w, web.PageData{
		Title:    pageTitle,
		ShareURL: shareURL,
		QRCode:   qr.Img(shareURL),
		FileName: meta.Name,
		FileSize: humanBytes(meta.Size),
		BlobHash: blobHash,
		Key:      keyText,
	})
}
func (a *Server) acquireUpload(w http.ResponseWriter, r *http.Request) bool {
	select {
	case a.uploads <- struct{}{}:
		return true
	case <-r.Context().Done():
		return false
	default:
		a.renderHomeError(w, "another upload is already in progress; try again in a moment")
		return false
	}
}

func (a *Server) releaseUpload() {
	<-a.uploads
}

func (a *Server) handleUploadRLNC(w http.ResponseWriter, r *http.Request, file io.Reader, meta cryptbox.FileMeta) {
	// 1. Chiffrer le flux dans un buffer temporaire
	tmp, err := os.CreateTemp("", "anonblossom-upload-rlnc-*")
	if err != nil {
		a.renderHomeError(w, "could not prepare encrypted upload")
		return
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()

	key, err := cryptbox.EncryptShare(file, tmp, meta, a.cfg.ChunkSize)
	if err != nil {
		a.renderHomeError(w, "encryption failed")
		return
	}
	defer badcrypt.Zeroize(key)

	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		a.renderHomeError(w, "could not read encrypted stream")
		return
	}
	encryptedData, err := io.ReadAll(tmp)
	if err != nil {
		a.renderHomeError(w, "could not buffer encrypted upload")
		return
	}

	// 2. Découper et générer les shards RLNC avec badrlnc
	k, n := rlncbox.CalculateKN(meta.Size)
	rlncShards, err := rlncbox.EncodeFile(encryptedData, k, n)
	if err != nil {
		a.renderHomeError(w, "RLNC sharding failed: "+err.Error())
		return
	}

	// 3. Sérialiser les shards et calculer les hashes
	serializedShards := make([][]byte, n)
	shardHashes := make([]string, n)
	for i, s := range rlncShards {
		serialized := rlncbox.SerializeShard(s)
		serializedShards[i] = serialized
		h := sha256.Sum256(serialized)
		shardHashes[i] = hex.EncodeToString(h[:])
	}

	// 4. Distribuer les N shards sur au moins 2 ou 3 relais distincts
	usedRelays, err := a.storage.UploadDistributedShards(r.Context(), serializedShards, shardHashes)
	if err != nil {
		a.renderHomeError(w, "failed to distribute shards: "+err.Error())
		return
	}

	// 5. Créer le Manifeste RLNC
	manifest := rlncbox.Manifest{
		Version:       1,
		FileSize:      meta.Size,
		EncryptedSize: int64(len(encryptedData)),
		ChunkSize:     len(rlncShards[0].Data),
		K:             k,
		N:             n,
		FileMeta:      meta,
		ShardHashes:   shardHashes,
		Relays:        usedRelays,
	}

	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		a.renderHomeError(w, "manifest generation failed")
		return
	}

	// 6. Chiffrer le manifeste avec la même clé de partage
	manifestMeta := cryptbox.FileMeta{
		Name: "manifest.json",
		Type: "application/x-anonblossom-manifest+json",
		Size: int64(len(manifestBytes)),
	}

	manifestTmp, err := os.CreateTemp("", "anonblossom-manifest-*")
	if err != nil {
		a.renderHomeError(w, "could not prepare manifest upload")
		return
	}
	defer os.Remove(manifestTmp.Name())
	defer manifestTmp.Close()

	if err := cryptbox.EncryptShareWithKey(key, bytes.NewReader(manifestBytes), manifestTmp, manifestMeta, 64<<10); err != nil {
		a.renderHomeError(w, "manifest encryption failed")
		return
	}

	manifestHash, manifestEncSize, err := fingerprint(manifestTmp)
	if err != nil {
		a.renderHomeError(w, "could not fingerprint manifest")
		return
	}

	// 7. Uploader le manifeste chiffré sur les relais de confiance
	if err := a.storage.Upload(r.Context(), manifestTmp, manifestEncSize, manifestHash); err != nil {
		a.renderHomeError(w, "manifest upload failed: "+err.Error())
		return
	}

	keyText := cryptbox.ShareKeyText(key)
	shareURL := a.publicBaseURL(r) + "/d/" + manifestHash + "/" + keyText

	w.Header().Set("Cache-Control", "no-store")
	_ = a.pages.Done(w, web.PageData{
		Title:    pageTitle,
		ShareURL: shareURL,
		QRCode:   qr.Img(shareURL),
		FileName: meta.Name,
		FileSize: humanBytes(meta.Size),
		BlobHash: manifestHash,
		Key:      keyText,
	})
}
