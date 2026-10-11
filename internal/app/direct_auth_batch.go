package app

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"anonblossom/internal/blossom"
	"anonblossom/internal/cryptbox"
)

const maxDirectAuthBatch = 8

type directBlobRequest struct {
	Hash string `json:"hash"`
	Size int64  `json:"size"`
}

type directBatchGrant struct {
	Hash    string                       `json:"hash"`
	Targets []blossom.DirectUploadTarget `json:"targets"`
}

// acquireDirectAuth bounds the number of concurrent signature batches. A
// nil limiter is used only by lightweight handler unit tests.
func (a *Server) acquireDirectAuth(w http.ResponseWriter) bool {
	if a.directAuthSlots == nil {
		return true
	}
	select {
	case a.directAuthSlots <- struct{}{}:
		return true
	default:
		w.Header().Set("Retry-After", "1")
		http.Error(w, "upload authorization capacity reached", http.StatusServiceUnavailable)
		return false
	}
}

func (a *Server) releaseDirectAuth() {
	if a.directAuthSlots != nil {
		<-a.directAuthSlots
	}
}

func (a *Server) checkDirectAuthRequest(w http.ResponseWriter, r *http.Request) bool {
	w.Header().Set("Cache-Control", "no-store")
	// Block cross-origin browser calls even if an app is accidentally served
	// behind a permissive proxy or with permissive CORS configuration.
	if site := r.Header.Get("Sec-Fetch-Site"); site == "cross-site" {
		http.Error(w, "cross-origin authorization denied", http.StatusForbidden)
		return false
	}
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
			return false
		}
	}
	if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		http.Error(w, "expected JSON", http.StatusUnsupportedMediaType)
		return false
	}
	return true
}

func (a *Server) validDirectBlob(b directBlobRequest) bool {
	return cryptbox.ValidHash(b.Hash) && b.Size >= 16 &&
		b.Size <= a.cfg.MaxUploadBytes+(1<<20)
}

func (a *Server) signDirectTargets(relays []string, hash string) ([]blossom.DirectUploadTarget, error) {
	targets := make([]blossom.DirectUploadTarget, 0, len(relays))
	for _, raw := range relays {
		cred, err := blossom.UploadAuthorization(raw, hash)
		if err != nil {
			return nil, err
		}
		targets = append(targets, blossom.DirectUploadTarget{URL: raw, Authorization: cred})
	}
	return targets, nil
}

// handleDirectUploadAuthBatch signs up to eight distinct, hash-scoped Blossom
// events in one OnionGuard-gated request. This amortizes gateway round trips
// without allowing arbitrary relay destinations or unbounded cryptographic CPU.
func (a *Server) handleDirectUploadAuthBatch(w http.ResponseWriter, r *http.Request) {
	if !a.checkDirectAuthRequest(w, r) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 2048)
	var req struct {
		Blobs []directBlobRequest `json:"blobs"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		http.Error(w, "invalid batch authorization request", http.StatusBadRequest)
		return
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		http.Error(w, "invalid batch authorization request", http.StatusBadRequest)
		return
	}
	if len(req.Blobs) == 0 || len(req.Blobs) > maxDirectAuthBatch {
		http.Error(w, "batch size must be 1 to 8", http.StatusBadRequest)
		return
	}
	seen := make(map[string]bool, len(req.Blobs))
	for _, b := range req.Blobs {
		if !a.validDirectBlob(b) || seen[b.Hash] {
			http.Error(w, "invalid or repeated blob request", http.StatusBadRequest)
			return
		}
		seen[b.Hash] = true
	}
	relays := a.directRelayURLs()
	if len(relays) < 2 {
		http.Error(w, "two configured Blossom relays are required", http.StatusServiceUnavailable)
		return
	}
	if !a.acquireDirectAuth(w) {
		return
	}
	defer a.releaseDirectAuth()
	grants := make([]directBatchGrant, 0, len(req.Blobs))
	for _, b := range req.Blobs {
		if r.Context().Err() != nil {
			return
		}
		targets, err := a.signDirectTargets(relays, b.Hash)
		if err != nil {
			http.Error(w, "unable to authorize upload", http.StatusServiceUnavailable)
			return
		}
		grants = append(grants, directBatchGrant{Hash: b.Hash, Targets: targets})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(struct {
		Grants []directBatchGrant `json:"grants"`
	}{Grants: grants})
}
