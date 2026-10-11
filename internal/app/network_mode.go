package app

import (
	"net/http"
	"net/url"
	"strings"

	"anonblossom/internal/cryptbox"
)

// A configured origin wins over request headers. Deploy separate clearnet and
// onion instances with their own PUBLIC_BASE_URL; never infer Tor from a UA.
func (a *Server) serverMode(r *http.Request) bool {
	origin := a.cfg.PublicBaseURL
	if origin == "" {
		origin = "http://" + r.Host
	}
	u, err := url.Parse(origin)
	return err == nil && strings.HasSuffix(strings.ToLower(u.Hostname()), ".onion")
}

func (a *Server) networkMode(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !a.serverMode(r) {
			if r.Method == http.MethodPost && (r.URL.Path == "/upload" || strings.HasPrefix(r.URL.Path, "/d/")) {
				http.Error(w, "Use local encryption on clearnet or the onion service for server processing.", http.StatusForbidden)
				return
			}
			// Preserve older server-generated links while moving subsequent
			// decryption to the browser. Their key already reached this server.
			if r.Method == http.MethodGet || r.Method == http.MethodHead {
				if hash, key, ok := cryptbox.ParseDownloadPath(r.URL.Path); ok {
					w.Header().Set("Cache-Control", "no-store")
					http.Redirect(w, r, "/d/"+hash+"#"+cryptbox.ShareKeyText(key), http.StatusSeeOther)
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

// A fragment key is never sent automatically. Onion recipients explicitly
// submit it, including manually when Tor's Safest mode disables JavaScript.
func (a *Server) handleOnionKey(w http.ResponseWriter, r *http.Request) {
	hash, ok := bareBlobHash(r.URL.Path)
	if !a.serverMode(r) || !ok {
		http.NotFound(w, r)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid decryption form", http.StatusBadRequest)
		return
	}
	path := "/d/" + hash + "/" + strings.TrimSpace(r.PostForm.Get("key"))
	if _, _, valid := cryptbox.ParseDownloadPath(path); !valid {
		http.Error(w, "invalid decryption key", http.StatusBadRequest)
		return
	}
	request := r.Clone(r.Context())
	request.Method = http.MethodGet
	request.URL.Path = path
	request.URL.RawQuery = ""
	a.handleDownload(w, request)
}
