package qr

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestImgReferencesPNGEndpoint(t *testing.T) {
	img := string(Img("https://example.org/share/test"))
	for _, needle := range []string{
		`<img `,
		`src="/qr?data=`,
		`alt="QR code"`,
		`width="256"`,
		`height="256"`,
	} {
		if !strings.Contains(img, needle) {
			t.Fatalf("QR img missing %q", needle)
		}
	}
	if strings.Contains(img, "<svg") {
		t.Fatal("QR markup must not embed SVG (blocked in Tor Browser Safest mode)")
	}
}

func TestHandlerServesStaticPNG(t *testing.T) {
	req := httptest.NewRequest("GET", "/qr?data=https%3A%2F%2Fexample.org%2Fshare%2Ftest", nil)
	rec := httptest.NewRecorder()
	Handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Content-Type"); got != "image/png" {
		t.Fatalf("Content-Type = %q, want image/png", got)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store, no-cache, must-revalidate" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
	body := rec.Body.Bytes()
	if len(body) < 8 || string(body[:8]) != "\x89PNG\r\n\x1a\n" {
		t.Fatal("response body is not a PNG")
	}
}

func TestHandlerRejectsMissingData(t *testing.T) {
	req := httptest.NewRequest("GET", "/qr", nil)
	rec := httptest.NewRecorder()
	Handler(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}
