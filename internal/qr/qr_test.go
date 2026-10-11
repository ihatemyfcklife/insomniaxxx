package qr

import (
	"bytes"
	"encoding/base64"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestImgEmbedsPNGWithoutLeakingKeyInURL(t *testing.T) {
	img := string(Img("https://example.org/share/test"))
	for _, needle := range []string{
		`<img `,
		`src="data:image/png;base64,`,
		`alt="QR code"`,
		`width="256"`,
		`height="256"`,
	} {
		if !strings.Contains(img, needle) {
			t.Fatalf("QR img missing %q", needle)
		}
	}
	if strings.Contains(img, "/qr?data=") || strings.Contains(img, "https://example.org/share/test") {
		t.Fatal("share key or URL leaked into an HTTP image source")
	}
	if strings.Contains(img, "<svg") {
		t.Fatal("QR markup must not embed SVG (blocked in Tor Browser Safest mode)")
	}
}

func TestQRCodeMatchesExactDarkThemeInEveryRenderingMode(t *testing.T) {
	const share = "https://example.org/d/cipher#secret"

	// Browser WebAssembly uses GeneratePNG, so it must have the same colors as
	// both the private inline No-JS image and the legacy HTTP endpoint.
	browserPNG, err := GeneratePNG(share)
	if err != nil {
		t.Fatal(err)
	}
	assertQRCodeThemeColors(t, browserPNG)

	markup := string(Img(share))
	const prefix = `src="data:image/png;base64,`
	start := strings.Index(markup, prefix)
	if start < 0 {
		t.Fatal("inline QR is missing its PNG data URI")
	}
	encoded := markup[start+len(prefix):]
	end := strings.Index(encoded, `"`)
	if end < 0 {
		t.Fatal("inline QR data URI is not terminated")
	}
	inlinePNG, err := base64.StdEncoding.DecodeString(encoded[:end])
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(inlinePNG, browserPNG) {
		t.Fatal("No-JS and WebAssembly QR PNGs must be identical")
	}

	req := httptest.NewRequest("GET", "/qr?data="+url.QueryEscape(share), nil)
	rec := httptest.NewRecorder()
	Handler(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("QR handler status = %d", rec.Code)
	}
	if !bytes.Equal(rec.Body.Bytes(), browserPNG) {
		t.Fatal("HTTP QR must use the same themed generator")
	}
}

func assertQRCodeThemeColors(t *testing.T, data []byte) {
	t.Helper()
	image, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	background := color.RGBA{R: 0x18, G: 0x1C, B: 0x25, A: 0xFF}
	foreground := color.RGBA{R: 0x52, G: 0x5F, B: 0x7A, A: 0xFF}
	var backgroundCount, foregroundCount int
	bounds := image.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			pixel := color.RGBAModel.Convert(image.At(x, y)).(color.RGBA)
			switch pixel {
			case background:
				backgroundCount++
			case foreground:
				foregroundCount++
			default:
				t.Fatalf("QR pixel has non-theme color at (%d,%d): %v", x, y, pixel)
			}
		}
	}
	if foregroundCount == 0 || backgroundCount == 0 {
		t.Fatal("QR must contain both themed foreground and background")
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
