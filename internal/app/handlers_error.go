package app

import (
	"net/http"
	"strings"

	"anonblossom/internal/web"
)

// renderError renders the dedicated styled error page or falls back to text/json
// for machine API or raw binary requests.
func (a *Server) renderError(w http.ResponseWriter, r *http.Request, status int, title, message string, detail ...string) {
	// If requested as raw ciphertext, raw blob, or if the client explicitly requests JSON,
	// return standard HTTP text error so machine/script clients are not broken.
	if r.URL.Query().Has("cipher") || r.URL.Query().Has("raw") || strings.Contains(r.Header.Get("Accept"), "application/json") {
		msg := message
		if msg == "" {
			msg = http.StatusText(status)
		}
		http.Error(w, msg, status)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)

	var extraDetail string
	if len(detail) > 0 {
		extraDetail = detail[0]
	}

	_ = a.pages.Error(w, web.PageData{
		Title:        pageTitle,
		StatusCode:   status,
		ErrorTitle:   title,
		ErrorMessage: message,
		ErrorDetail:  extraDetail,
	})
}
