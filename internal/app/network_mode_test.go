package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"anonblossom/internal/config"
	"anonblossom/internal/cryptbox"
	"anonblossom/internal/web"
)

func TestNetworkModeSeparatesPlaintextProcessing(t *testing.T) {
	for _, origin := range []string{"https://files.example", "http://service.onion"} {
		a := &Server{cfg: config.Config{PublicBaseURL: origin}}
		called := false
		handler := a.networkMode(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }))
		r := httptest.NewRequest(http.MethodPost, "http://spoofed.onion/upload", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		wantServer := strings.HasSuffix(origin, ".onion")
		if called != wantServer || (!wantServer && w.Code != http.StatusForbidden) {
			t.Fatalf("origin %s: called=%v status=%d", origin, called, w.Code)
		}
	}
}

func TestNetworkModeRedirectsLegacyClearnetLinksWithoutDecrypting(t *testing.T) {
	a := &Server{cfg: config.Config{PublicBaseURL: "https://files.example"}}
	hash, key := strings.Repeat("a", 64), strings.Repeat("A", 43)
	w := httptest.NewRecorder()
	a.networkMode(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("server decryption reached") })).ServeHTTP(w,
		httptest.NewRequest(http.MethodGet, "/d/"+hash+"/"+key+"?download=1", nil))
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/d/"+hash+"#"+key {
		t.Fatalf("unexpected local-decryption redirect: %d %s", w.Code, w.Header().Get("Location"))
	}
}

func TestOnionHomeDoesNotLoadWasmAndClearnetStartsDisabled(t *testing.T) {
	pages, err := web.NewPages()
	if err != nil {
		t.Fatal(err)
	}
	for _, server := range []bool{false, true} {
		w := httptest.NewRecorder()
		if err := pages.Home(w, web.PageData{ServerMode: server}); err != nil {
			t.Fatal(err)
		}
		body := w.Body.String()
		if strings.Contains(body, "/static/boot.js") == server {
			t.Fatal("wrong upload engine rendered")
		}
		if strings.Contains(body, "class=\"btn-action\" disabled") == server {
			t.Fatal("wrong initial upload availability")
		}
	}
	w := httptest.NewRecorder()
	if err := pages.View(w, web.PageData{ServerMode: true, ZK: true, BlobHash: strings.Repeat("a", 64)}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(w.Body.String(), "/static/wasm_exec.js") || !strings.Contains(w.Body.String(), "Decrypt on the server") {
		t.Fatal("onion fragment share must require deliberate key submission without WASM")
	}
}

func TestOnionExplicitKeySubmissionDecryptsExistingShare(t *testing.T) {
	a, _, cleanup := setupTestServer(t)
	defer cleanup()
	defer a.Close()
	a.cfg.PublicBaseURL = "http://service.onion"
	var encrypted bytes.Buffer
	key, err := cryptbox.EncryptShare(strings.NewReader("hello"), &encrypted, cryptbox.FileMeta{Name: "fixture.txt", Type: "text/plain", Size: 5}, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(encrypted.Bytes())
	hash := hex.EncodeToString(sum[:])
	if err := a.storage.Upload(context.Background(), bytes.NewReader(encrypted.Bytes()), int64(encrypted.Len()), hash); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPost, "/d/"+hash, strings.NewReader("key="+cryptbox.ShareKeyText(key)))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	a.handleOnionKey(w, r)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "fixture.txt") || strings.Contains(w.Body.String(), "/static/wasm_exec.js") {
		t.Fatalf("onion server did not render decrypted metadata: %d", w.Code)
	}
}
