package app

import (
	"net/http"

	"anonblossom/internal/web"
)

const pageTitle = "insomniaXXX"

func (a *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		a.renderError(w, r, http.StatusNotFound, "Page Not Found", "The requested page does not exist or may have been moved.")
		return
	}

	w.Header().Set("Cache-Control", "no-store")
	_ = a.pages.Home(w, web.PageData{
		Title:     pageTitle,
		MaxUpload: humanBytes(a.cfg.MaxUploadBytes),
		MaxUploadBytes: a.cfg.MaxUploadBytes,
	})
}
