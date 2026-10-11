package app

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"anonblossom/internal/cryptbox"
)

func TestErrorPageOnUnknownRoutes(t *testing.T) {
	server, _, cleanup := setupTestServer(t)
	defer cleanup()
	server.admission = nil

	routes := server.routes()

	// 1. GET unknown route should return 404 HTML error page
	req := httptest.NewRequest(http.MethodGet, "/random-unknown-page", nil)
	rec := httptest.NewRecorder()
	routes.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}

	body := rec.Body.String()
	for _, needle := range []string{
		"404",
		"Page Not Found",
		`class="error-card"`,
		`class="logo-box"`,
		`href="/"`,
		`href="/relays"`,
	} {
		if !strings.Contains(body, needle) {
			t.Fatalf("error page missing %q in body:\n%s", needle, body)
		}
	}

	// 2. POST to unknown route should also return 404 HTML error page
	postReq := httptest.NewRequest(http.MethodPost, "/non-existent-action", bytes.NewReader([]byte("data")))
	postRec := httptest.NewRecorder()
	routes.ServeHTTP(postRec, postReq)

	if postRec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown POST, got %d", postRec.Code)
	}
	if !strings.Contains(postRec.Body.String(), `class="error-card"`) {
		t.Fatalf("expected error-card in POST 404 response")
	}

	// 3. GET malformed download path should return 404 HTML error page
	dlReq := httptest.NewRequest(http.MethodGet, "/d/invalid-path", nil)
	dlRec := httptest.NewRecorder()
	routes.ServeHTTP(dlRec, dlReq)

	if dlRec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for invalid download path, got %d", dlRec.Code)
	}
	if !strings.Contains(dlRec.Body.String(), "Invalid Link") {
		t.Fatalf("expected 'Invalid Link' in response, got:\n%s", dlRec.Body.String())
	}

	testHash := strings.Repeat("a", 64)
	testKey := cryptbox.ShareKeyText(make([]byte, 32))

	// Server-side error cases belong to the onion processing instance.
	server.cfg.PublicBaseURL = "http://service.onion"

	// 4. GET raw query on not found should return plain text error
	rawReq := httptest.NewRequest(http.MethodGet, "/d/"+testHash+"/"+testKey+"?raw=1", nil)
	rawRec := httptest.NewRecorder()
	routes.ServeHTTP(rawRec, rawReq)

	if rawRec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing raw blob, got %d", rawRec.Code)
	}
	if strings.Contains(rawRec.Body.String(), "<html") {
		t.Fatalf("expected plain text for raw request, got HTML")
	}

	// 5. GET missing file in storage should return 404 HTML error page
	missingReq := httptest.NewRequest(http.MethodGet, "/d/"+testHash+"/"+testKey, nil)
	missingRec := httptest.NewRecorder()
	routes.ServeHTTP(missingRec, missingReq)

	if missingRec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing file, got %d", missingRec.Code)
	}
	if !strings.Contains(missingRec.Body.String(), "File Not Found") {
		t.Fatalf("expected 'File Not Found' in HTML error page, got:\n%s", missingRec.Body.String())
	}

	// 6. Test renderError with 502 and custom detail
	errRec := httptest.NewRecorder()
	errReq := httptest.NewRequest(http.MethodGet, "/test-err", nil)
	server.renderError(errRec, errReq, http.StatusBadGateway, "Storage Unavailable", "Relays failed to respond.", "connection refused: 10.0.0.1:8080")

	if errRec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d", errRec.Code)
	}
	errBody := errRec.Body.String()
	for _, needle := range []string{
		"502",
		"Storage Unavailable",
		"Relays failed to respond.",
		"connection refused: 10.0.0.1:8080",
		`class="error-detail-box"`,
	} {
		if !strings.Contains(errBody, needle) {
			t.Fatalf("expected %q in 502 error body", needle)
		}
	}
}

func TestFaviconRoutes(t *testing.T) {
	server, _, cleanup := setupTestServer(t)
	defer cleanup()
	server.admission = nil

	routes := server.routes()

	// 1. GET /favicon.ico
	reqICO := httptest.NewRequest(http.MethodGet, "/favicon.ico", nil)
	recICO := httptest.NewRecorder()
	routes.ServeHTTP(recICO, reqICO)

	if recICO.Code != http.StatusOK {
		t.Fatalf("expected 200 for /favicon.ico, got %d", recICO.Code)
	}
	if recICO.Body.Len() == 0 {
		t.Fatal("/favicon.ico body is empty")
	}

	// 2. GET /static/favicon.png
	reqPNG := httptest.NewRequest(http.MethodGet, "/static/favicon.png", nil)
	recPNG := httptest.NewRecorder()
	routes.ServeHTTP(recPNG, reqPNG)

	if recPNG.Code != http.StatusOK {
		t.Fatalf("expected 200 for /static/favicon.png, got %d", recPNG.Code)
	}
	if !bytes.HasPrefix(recPNG.Body.Bytes(), []byte("\x89PNG")) {
		t.Fatal("/static/favicon.png does not have PNG header")
	}

	// 3. GET /static/favicon.svg
	reqSVG := httptest.NewRequest(http.MethodGet, "/static/favicon.svg", nil)
	recSVG := httptest.NewRecorder()
	routes.ServeHTTP(recSVG, reqSVG)

	if recSVG.Code != http.StatusOK {
		t.Fatalf("expected 200 for /static/favicon.svg, got %d", recSVG.Code)
	}
	if !strings.Contains(recSVG.Body.String(), "<svg") {
		t.Fatal("/static/favicon.svg does not contain <svg")
	}

	// 4. GET /static/logo.png
	reqLogo := httptest.NewRequest(http.MethodGet, "/static/logo.png", nil)
	recLogo := httptest.NewRecorder()
	routes.ServeHTTP(recLogo, reqLogo)

	if recLogo.Code != http.StatusOK {
		t.Fatalf("expected 200 for /static/logo.png, got %d", recLogo.Code)
	}
	if !bytes.HasPrefix(recLogo.Body.Bytes(), []byte("\x89PNG")) {
		t.Fatal("/static/logo.png does not have PNG header")
	}

	// 5. GET /static/xxx.png
	reqXXX := httptest.NewRequest(http.MethodGet, "/static/xxx.png", nil)
	recXXX := httptest.NewRecorder()
	routes.ServeHTTP(recXXX, reqXXX)

	if recXXX.Code != http.StatusOK {
		t.Fatalf("expected 200 for /static/xxx.png, got %d", recXXX.Code)
	}
	if !bytes.HasPrefix(recXXX.Body.Bytes(), []byte("\x89PNG")) {
		t.Fatal("/static/xxx.png does not have PNG header")
	}
}
