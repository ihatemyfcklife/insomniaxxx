package app

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"anonblossom/internal/cryptbox"
	"anonblossom/internal/web"
)

func singleFile(files []*multipart.FileHeader) (*multipart.FileHeader, bool) {
	if len(files) != 1 {
		return nil, false
	}
	return files[0], true
}

func uploadFilename(name string) string {
	name = cryptbox.CleanFilename(name)
	if name == "" {
		return "download"
	}
	return name
}

func contentType(filename, provided string) string {
	if provided != "" {
		if mediaType, _, err := mime.ParseMediaType(provided); err == nil && mediaType != "" {
			return mediaType
		}
	}
	if mediaType := mime.TypeByExtension(strings.ToLower(filepath.Ext(filename))); mediaType != "" {
		return mediaType
	}
	return "application/octet-stream"
}

func fingerprint(file *os.File) (hash string, size int64, err error) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", 0, err
	}
	info, err := file.Stat()
	if err != nil {
		return "", 0, err
	}

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(hasher.Sum(nil)), info.Size(), nil
}

func (a *Server) renderHomeError(w http.ResponseWriter, message string) {
	w.WriteHeader(http.StatusBadRequest)
	_ = a.pages.Home(w, web.PageData{Title: pageTitle, Error: cleanError(message)})
}

func cleanError(message string) string {
	message = strings.TrimSpace(message)
	if message == "" {
		return "request failed"
	}
	if len(message) > 512 {
		return fmt.Sprintf("%s…", message[:511])
	}
	return message
}
