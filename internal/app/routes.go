package app

import (
	"embed"
	"io/fs"
	"net/http"

	"anonblossom/internal/qr"
)

//go:embed static/*
var staticFS embed.FS

func (a *Server) routes() http.Handler {
	// 1. Routeur dédié aux assets statiques (NON protégé par OnionGuard)
	staticMux := http.NewServeMux()
	subFS, err := fs.Sub(staticFS, "static")
	if err == nil {
		staticMux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(subFS))))
	}

	// 2. Routeur dédié à l'application (PROTÉGÉ par OnionGuard)
	appMux := http.NewServeMux()
	appMux.HandleFunc("GET /", a.handleHome)
	appMux.HandleFunc("GET /qr", qr.Handler)
	appMux.HandleFunc("GET /relays", a.handleRelays)
	appMux.HandleFunc("GET /upload", redirectHome)
	appMux.HandleFunc("POST /upload", a.handleUpload)
	appMux.HandleFunc("POST /upload-raw", a.handleUploadRaw)
	appMux.HandleFunc("GET /d/", a.handleDownload)
	muxWrapper := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h, pattern := appMux.Handler(r)
		if pattern == "" {
			a.renderError(w, r, http.StatusNotFound, "Page Not Found", "The requested page does not exist or may have been moved.")
			return
		}
		h.ServeHTTP(w, r)
	})

	protectedApp := a.admission.Middleware(muxWrapper)

	// 3. Routeur racine qui aiguille les requêtes
	root := http.NewServeMux()

	// Les requêtes /static/ vont directement vers le serveur de fichiers sans passer par le sas
	root.Handle("GET /static/", staticMux)

	// Favicon servi directement à la racine sans passer par le sas
	root.HandleFunc("GET /favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFileFS(w, r, subFS, "favicon.ico")
	})

	// Tout le reste passe par OnionGuard
	root.Handle("/", protectedApp)

	// Security headers apply to every response, including static assets and
	// favicon routes which intentionally bypass OnionGuard admission.
	return securityHeaders(root)
}

func redirectHome(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
