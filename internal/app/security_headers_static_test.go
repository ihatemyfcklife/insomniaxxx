package app

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSecurityHeadersProtectStaticAndFavicon(t *testing.T) {
	server, _, cleanup := setupTestServer(t)
	defer cleanup()
	server.admission = nil
	for _, path := range []string{"/static/boot.js", "/favicon.ico"} {
		t.Run(path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, path, nil)
			server.routes().ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("asset response: %d", rec.Code)
			}
			if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
				t.Fatalf("missing nosniff on %s: %q", path, got)
			}
			if got := rec.Header().Get("Referrer-Policy"); got != "no-referrer" {
				t.Fatalf("missing referrer protection on %s: %q", path, got)
			}
			if got := rec.Header().Get("Content-Security-Policy"); !strings.Contains(got, "default-src 'none'") {
				t.Fatalf("missing CSP on %s: %q", path, got)
			}
		})
	}
}
