package app

import (
	"strings"
	"testing"
)

// Regression: in-browser Zero-Knowledge share keys are in URL fragments;
// /qr?data= would send those fragments back to the server.
func TestZeroKnowledgeQRDoesNotSendShareKeyToServer(t *testing.T) {
	content, err := staticFS.ReadFile("static/boot.js")
	if err != nil { t.Fatal(err) }
	src := string(content)
	if strings.Contains(src, `"/qr?data="`) ||
		strings.Contains(src, "const qrEndpoint") {
		t.Fatal("browser QR generation leaks fragment-bearing links to /qr")
	}
	if !strings.Contains(src, "wasmGenerateQR(shareURL)") {
		t.Fatal("missing local QR generation")
	}
	if !strings.Contains(src, "shards[0].length - 40") {
		t.Fatal("WASM manifest must record RLNC data size without wire header")
	}
}

func TestZKShardChecksContentHashesAndLimitsConcurrency(t *testing.T) {
	content, err := staticFS.ReadFile("static/view.js")
	if err != nil { t.Fatal(err) }
	src := string(content)
	for _, needle := range []string{"wasmSHA256(buf) !== hash", "Promise.all([worker(), worker()])", "?cipher=1"} {
		if !strings.Contains(src, needle) { t.Fatalf("missing protection %q", needle) }
	}
	if strings.Contains(src, "candidateRelays") {
		t.Fatal("browser should not fetch unknown cross-origin relays")
	}
}
