package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/ihatemyfcklife/badcrypt"

	"anonblossom/internal/blossom"
	"anonblossom/internal/cryptbox"
	"anonblossom/internal/rlncbox"
	"anonblossom/internal/web"
)

func (a *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
	if !a.acquireDownload(w) {
		return
	}
	defer a.releaseDownload()

	hash, key, ok := cryptbox.ParseDownloadPath(r.URL.Path)
	if !ok {
		// URL Zero-Knowledge : /d/<hash>#<clé> — la clé reste dans le fragment.
		if bareHash, isBare := bareBlobHash(r.URL.Path); isBare {
			a.handleBareBlob(w, r, bareHash)
			return
		}
		a.renderError(w, r, http.StatusNotFound, "Invalid Link", "The download link is invalid or incomplete.")
		return
	}
	defer badcrypt.Zeroize(key)

	// Mode 1 : Page de prévisualisation HTML dédiée
	if !r.URL.Query().Has("raw") && !r.URL.Query().Has("download") {
		response, err := a.storage.Fetch(r.Context(), hash)
		if err != nil {
			if errors.Is(err, blossom.ErrNotFound) {
				a.renderError(w, r, http.StatusNotFound, "File Not Found", "The requested file does not exist on the configured Blossom relays or has expired.")
				return
			}
			a.renderError(w, r, http.StatusBadGateway, "Storage Unavailable", "Could not reach Blossom relays to retrieve this file.")
			return
		}
		defer response.Body.Close()

		blobBytes, err := readLimitedBlob(response.Body, a.maxStoredBlobBytes())
		if err != nil {
			a.renderError(w, r, http.StatusBadGateway, "Transfer Error", "Failed to read the encrypted file from storage.")
			return
		}

		meta, err := cryptbox.InspectShare(bytes.NewReader(blobBytes), key)
		if err != nil {
			a.renderError(w, r, http.StatusBadRequest, "Invalid Encrypted Share", "The file could not be verified. The decryption key may be incorrect or the share is corrupted.")
			return
		}

		// Si les métadonnées indiquent un manifeste RLNC :
		if meta.Type == "application/x-anonblossom-manifest+json" {
			var manifestBuf bytes.Buffer
			if _, decErr := cryptbox.DecryptTo(&manifestBuf, bytes.NewReader(blobBytes), int64(len(blobBytes)), key, 1<<20); decErr == nil {
				var manifest rlncbox.Manifest
				if jsonErr := json.Unmarshal(manifestBuf.Bytes(), &manifest); jsonErr == nil && manifest.Version == 1 {
					meta = manifest.FileMeta
				}
			}
		}

		isImg := safeInlineType(meta.Type, "image/")
		isVideo := safeInlineType(meta.Type, "video/")
		isAudio := safeInlineType(meta.Type, "audio/")

		w.Header().Set("Cache-Control", "no-store")
		_ = a.pages.View(w, web.PageData{
			Title:       pageTitle,
			FileName:    meta.Name,
			FileSize:    humanBytes(meta.Size),
			FileType:    meta.Type,
			RawURL:      r.URL.Path + "?raw=1",
			DownloadURL: r.URL.Path + "?download=1",
			IsImage:     isImg,
			IsVideo:     isVideo,
			IsAudio:     isAudio,
		})
		return
	}

	// Mode 2 : Déchiffrement et streaming avec cache éphémère
	cacheKey := hash + ":" + cryptbox.ShareKeyText(key)

	file, meta, err := a.cache.GetOrDecrypt(r.Context(), cacheKey, func() (string, cryptbox.FileMeta, error) {
		response, err := a.storage.Fetch(r.Context(), hash)
		if err != nil {
			return "", cryptbox.FileMeta{}, err
		}
		defer response.Body.Close()

		blobBytes, err := readLimitedBlob(response.Body, a.maxStoredBlobBytes())
		if err != nil {
			return "", cryptbox.FileMeta{}, err
		}

		inspectMeta, inspectErr := cryptbox.InspectShare(bytes.NewReader(blobBytes), key)
		if inspectErr == nil && inspectMeta.Type == "application/x-anonblossom-manifest+json" {
			// C'EST UN MANIFESTE RLNC !
			var manifestBuf bytes.Buffer
			_, decErr := cryptbox.DecryptTo(&manifestBuf, bytes.NewReader(blobBytes), int64(len(blobBytes)), key, 1<<20)
			if decErr != nil {
				return "", cryptbox.FileMeta{}, decErr
			}
			var manifest rlncbox.Manifest
			if jsonErr := json.Unmarshal(manifestBuf.Bytes(), &manifest); jsonErr != nil || !validRLNCManifest(manifest, a.cfg.MaxUploadBytes) {
				return "", cryptbox.FileMeta{}, errors.New("invalid rlnc manifest")
			}

			// Téléchargement parallèle de K shards innovants parmi les relais
			shards, fetchErr := a.storage.FetchInnovativeShards(r.Context(), manifest.ShardHashes, manifest.Relays, manifest.K, int64(manifest.ChunkSize)+40)
			if fetchErr != nil {
				return "", cryptbox.FileMeta{}, fetchErr
			}

			// Décodage RLNC via badrlnc (rlncbox.DecodeFile)
			encryptedStream, decodeErr := rlncbox.DecodeFile(shards, manifest.K, int(manifest.EncryptedSize))
			if decodeErr != nil {
				return "", cryptbox.FileMeta{}, decodeErr
			}

			// Déchiffrement du flux reconstitué dans un fichier temporaire
			tmp, tmpErr := os.CreateTemp("", "anonblossom-cache-*")
			if tmpErr != nil {
				return "", cryptbox.FileMeta{}, tmpErr
			}
			tmpName := tmp.Name()

			meta, decFileErr := cryptbox.DecryptTo(tmp, bytes.NewReader(encryptedStream), int64(len(encryptedStream)), key, a.cfg.MaxUploadBytes)
			_ = tmp.Close()
			if decFileErr != nil {
				_ = os.Remove(tmpName)
				return "", cryptbox.FileMeta{}, decFileErr
			}
			return tmpName, meta, nil
		}

		// BLOB MONOLITHIQUE STANDARD (non-manifeste)
		tmp, tmpErr := os.CreateTemp("", "anonblossom-cache-*")
		if tmpErr != nil {
			return "", cryptbox.FileMeta{}, tmpErr
		}
		tmpName := tmp.Name()

		meta, decErr := cryptbox.DecryptTo(tmp, bytes.NewReader(blobBytes), int64(len(blobBytes)), key, a.cfg.MaxUploadBytes)
		_ = tmp.Close()
		if decErr != nil {
			_ = os.Remove(tmpName)
			return "", cryptbox.FileMeta{}, decErr
		}
		return tmpName, meta, nil
	})

	if err != nil {
		if errors.Is(err, blossom.ErrNotFound) {
			a.renderError(w, r, http.StatusNotFound, "File Not Found", "The requested file does not exist on the configured Blossom relays or has expired.")
			return
		}
		a.renderError(w, r, http.StatusBadGateway, "Download Unavailable", "Could not retrieve or decode the file from Blossom relays.")
		return
	}
	defer file.Close()

	w.Header().Set("Content-Type", meta.Type)
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")

	if r.URL.Query().Has("download") || !safeInlineType(meta.Type, "") {
		// Untrusted HTML, SVG, XML or PDF must never render as active content
		// on the application's own origin.
		w.Header().Set("Content-Disposition", cryptbox.ContentDisposition(meta.Name))
	} else {
		w.Header().Set("Content-Disposition", "inline")
	}

	http.ServeContent(w, r, meta.Name, time.Time{}, file)
}

// Only passive formats may render directly on the application's origin.
func safeInlineType(raw, prefix string) bool {
	switch strings.ToLower(strings.TrimSpace(strings.Split(raw, ";")[0])) {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
		return prefix == "" || prefix == "image/"
	case "audio/mpeg", "audio/ogg", "audio/wav", "audio/webm":
		return prefix == "" || prefix == "audio/"
	case "video/mp4", "video/webm", "video/ogg":
		return prefix == "" || prefix == "video/"
	}
	return false
}

// validRLNCManifest bounds all attacker-controlled parameters before shard downloads/decoding.
func validRLNCManifest(m rlncbox.Manifest, maxFileSize int64) bool {
	if m.Version != 1 || m.FileSize <= rlncbox.Threshold || m.FileSize > maxFileSize ||
		m.EncryptedSize <= 0 || m.EncryptedSize > maxFileSize+(4<<20) ||
		m.K <= 0 || m.K > 64 || m.N < m.K || m.N > 96 ||
		len(m.ShardHashes) != m.N || len(m.Relays) > 50 ||
		m.FileMeta.Size != m.FileSize {
		return false
	}
	expectedK, expectedN := rlncbox.CalculateKN(m.FileSize)
	if m.K != expectedK || m.N != expectedN {
		return false
	}
	expectedChunkSize := (m.EncryptedSize + int64(m.K) - 1) / int64(m.K)
	if m.ChunkSize <= 0 || int64(m.ChunkSize) != expectedChunkSize {
		return false
	}
	for _, hash := range m.ShardHashes {
		if !cryptbox.ValidHash(hash) {
			return false
		}
	}
	return true
}

// bareBlobHash accepte les chemins /d/<hash> sans segment de clé.
func bareBlobHash(path string) (string, bool) {
	const prefix = "/d/"
	if !strings.HasPrefix(path, prefix) {
		return "", false
	}
	hash := strings.TrimPrefix(path, prefix)
	if strings.Contains(hash, "/") || !cryptbox.ValidHash(hash) {
		return "", false
	}
	return hash, true
}

// handleBareBlob sert les URLs Zero-Knowledge : page de vue en mode Wasm, ou
// relais du chiffré brut (?cipher=1) pour un déchiffrement 100 % navigateur.
func (a *Server) handleBareBlob(w http.ResponseWriter, r *http.Request, hash string) {
	if r.URL.Query().Has("cipher") {
		response, err := a.storage.Fetch(r.Context(), hash)
		if err != nil {
			if errors.Is(err, blossom.ErrNotFound) {
				http.Error(w, "file not found", http.StatusNotFound)
				return
			}
			http.Error(w, "storage unavailable", http.StatusBadGateway)
			return
		}
		defer response.Body.Close()

		blobBytes, readErr := readLimitedBlob(response.Body, a.maxStoredBlobBytes())
		if readErr != nil {
			http.Error(w, "stored blob exceeds the configured size limit", http.StatusBadGateway)
			return
		}

		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Cache-Control", "private, no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		w.Header().Set("Content-Disposition", "inline")
		_, _ = w.Write(blobBytes)
		return
	}

	w.Header().Set("Cache-Control", "no-store")
	_ = a.pages.View(w, web.PageData{
		Title:     pageTitle,
		FileName:  "Encrypted file",
		FileSize:  "—",
		FileType:  "—",
		ZK:        true,
		CipherURL: "/d/" + hash + "?cipher=1",
	})
}

func (a *Server) acquireDownload(w http.ResponseWriter) bool {
	if a.downloads == nil {
		http.Error(w, "download capacity is unavailable", http.StatusServiceUnavailable)
		return false
	}
	select {
	case a.downloads <- struct{}{}:
		return true
	default:
		http.Error(w, "too many concurrent downloads; try again shortly", http.StatusServiceUnavailable)
		return false
	}
}

func (a *Server) releaseDownload() {
	if a.downloads != nil {
		<-a.downloads
	}
}

// maxStoredBlobBytes bounds untrusted relay responses to the maximum encrypted object size.
func (a *Server) maxStoredBlobBytes() int64 {
	return a.cfg.MaxUploadBytes + (4 << 20)
}

// readLimitedBlob reads no more than limit+1 bytes so oversized or chunked relay
// responses cannot grow memory without bound or be mistaken for complete objects.
func readLimitedBlob(r io.Reader, limit int64) ([]byte, error) {
	if verified, ok := r.(interface{ VerifiedBytes() []byte }); ok {
		data := verified.VerifiedBytes()
		if limit <= 0 || int64(len(data)) > limit {
			return nil, errors.New("blob exceeds configured size limit")
		}
		return data, nil
	}
	if limit <= 0 {
		return nil, errors.New("invalid blob size limit")
	}
	data, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errors.New("blob exceeds configured size limit")
	}
	return data, nil
}
