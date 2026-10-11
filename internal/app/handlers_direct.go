package app

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"

	"anonblossom/internal/blossom"
	"anonblossom/internal/cryptbox"
)

// directRelayURLs returns only explicitly configured storage origins.
// Never expose discovered/untrusted relay URLs as credential destinations.
func (a *Server) directRelayURLs() []string {
	urls := make([]string, 0, 3)
	seen := make(map[string]bool)
	for _, raw := range a.cfg.BlossomURLs {
		u, err := url.Parse(raw)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Hostname() == "" || u.User != nil ||
			u.Path != "" && u.Path != "/" || u.RawQuery != "" || u.Fragment != "" {
			continue
		}
		if u.Scheme != "https" && a.cfg.PublicBaseURL != "" && strings.HasPrefix(a.cfg.PublicBaseURL, "https://") {
			continue
		}
		host := strings.ToLower(u.Hostname())
		if seen[host] {
			continue
		}
		seen[host] = true
		urls = append(urls, strings.TrimRight(raw, "/"))
		if len(urls) == 3 {
			break
		}
	}
	return urls
}

// handleDirectRelays publishes a *restricted* browser-visible list of
// configured Blossom origins, not the full discovered/manifest relay pool.
func (a *Server) handleDirectRelays(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(struct {
		Relays []string `json:"relays"`
	}{Relays: a.directRelayURLs()})
}

// handleDirectUploadAuth mints short-lived, single-blob BUD-02 authorizations.
// No plaintext, encryption keys, or blob contents are sent to this endpoint.
// The OnionGuard gate protects the endpoint and its origin set is fixed.
func (a *Server) handleDirectUploadAuth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if origin := r.Header.Get("Origin"); origin != "" {
		expected := a.cfg.PublicBaseURL
		if expected == "" {
			expected = "http://" + r.Host
			if r.TLS != nil {
				expected = "https://" + r.Host
			}
		}
		if origin != expected {
			http.Error(w, "cross-origin authorization denied", http.StatusForbidden)
			return
		}
	}
	if contentType := r.Header.Get("Content-Type"); !strings.HasPrefix(contentType, "application/json") {
		http.Error(w, "expected JSON", http.StatusUnsupportedMediaType)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 512)
	var req struct {
		Hash string `json:"hash"`
		Size int64  `json:"size"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		http.Error(w, "invalid authorization request", http.StatusBadRequest)
		return
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		http.Error(w, "invalid authorization request", http.StatusBadRequest)
		return
	}
	if !cryptbox.ValidHash(req.Hash) || req.Size < 16 || req.Size > a.cfg.MaxUploadBytes+(1<<20) {
		http.Error(w, "invalid blob hash or size", http.StatusBadRequest)
		return
	}
	urls := a.directRelayURLs()
	if len(urls) < 2 {
		http.Error(w, "two configured Blossom relays are required", http.StatusServiceUnavailable)
		return
	}
	targets := make([]blossom.DirectUploadTarget, 0, len(urls))
	for _, raw := range urls {
		cred, err := blossom.UploadAuthorization(raw, req.Hash)
		if err != nil {
			http.Error(w, "unable to authorize upload", http.StatusServiceUnavailable)
			return
		}
		targets = append(targets, blossom.DirectUploadTarget{URL: raw, Authorization: cred})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(struct {
		Targets []blossom.DirectUploadTarget `json:"targets"`
	}{Targets: targets})
}
