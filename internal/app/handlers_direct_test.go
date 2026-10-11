package app

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"anonblossom/internal/config"
)

func directTestServer() *Server {
	return &Server{cfg: config.Config{
		PublicBaseURL: "https://files.example",
		MaxUploadBytes: 19 << 20,
		BlossomURLs: []string{
			"https://first.example",
			"https://second.example",
			"https://first.example:8443",
			"https://third.example",
		},
	}}
}

func TestDirectRelaysConfiguredOnlyDistinctHosts(t *testing.T) {
	a := directTestServer()
	got := a.directRelayURLs()
	if len(got) != 3 || got[0] != "https://first.example" ||
		got[1] != "https://second.example" || got[2] != "https://third.example" {
		t.Fatalf("untrusted or duplicated browser relays: %q", got)
	}
	w := httptest.NewRecorder()
	a.handleDirectRelays(w, httptest.NewRequest("GET", "/direct-relays", nil))
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" ||
		strings.Contains(w.Body.String(), "8443") {
		t.Fatalf("invalid published relay list: %d %s", w.Code, w.Body.String())
	}
}

func TestDirectUploadAuthIsHashBoundAndRejectsCrossOrigin(t *testing.T) {
	a := directTestServer()
	hash := strings.Repeat("a", 64)
	authReq := func(body string, origin string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/direct-upload-auth", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		if origin != "" { r.Header.Set("Origin", origin) }
		w := httptest.NewRecorder()
		a.handleDirectUploadAuth(w, r)
		return w
	}
	good := authReq(`{"hash":"`+hash+`","size":128}`, "https://files.example")
	if good.Code != http.StatusOK { t.Fatalf("expected auth, got %d: %s", good.Code, good.Body) }
	var data struct {
		Targets []struct { URL string `json:"url"`; Authorization string `json:"authorization"` } `json:"targets"`
	}
	if err := json.Unmarshal(good.Body.Bytes(), &data); err != nil { t.Fatal(err) }
	if len(data.Targets) != 3 { t.Fatalf("expected 3 distinct configured targets, got %d", len(data.Targets)) }
	for _, target := range data.Targets {
		if !strings.HasPrefix(target.Authorization, "Nostr ") { t.Fatal("missing Blossom authorization") }
		serialized, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(target.Authorization, "Nostr "))
		if err != nil { t.Fatal(err) }
		var evt struct {
			Kind int `json:"kind"`
			Tags [][]string `json:"tags"`
		}
		if err := json.Unmarshal(serialized, &evt); err != nil { t.Fatal(err) }
		if evt.Kind != 24242 { t.Fatal("wrong authorization event type") }
		foundHash, foundHost := false, false
		for _, tag := range evt.Tags {
			if len(tag) < 2 { continue }
			if tag[0]=="x" && tag[1]==hash { foundHash=true }
			if tag[0]=="server" && strings.Contains(target.URL, tag[1]) { foundHost=true }
		}
		if !foundHash || !foundHost { t.Fatalf("credential is not scoped to hash or host: %q", target.URL) }
	}
	for _, tc := range []struct{body, origin string}{
		{`{"hash":"`+hash+`","size":128}`, "https://evil.example"},
		{`{"hash":"bad","size":128}`, ""},
		{`{"hash":"`+hash+`","size":0}`, ""},
		{`{"hash":"`+hash+`","size":100000000}`, ""},
		{`{"hash":"`+hash+`","size":128,"url":"http://127.0.0.1"}`, ""},
		{`{"hash":"`+hash+`","size":128} {}`, ""},
		{strings.Repeat(" ", 600), ""},
	} {
		w := authReq(tc.body, tc.origin)
		if w.Code == http.StatusOK { t.Fatalf("accepted invalid request: %q", tc.body[:min(len(tc.body), 100)]) }
	}
}

func TestDirectAuthFailsClosedWithoutTwoTargets(t *testing.T) {
	a := directTestServer()
	a.cfg.BlossomURLs = []string{"https://one.example"}
	r := httptest.NewRequest("POST", "/direct-upload-auth", bytes.NewBufferString(`{"hash":"`+strings.Repeat("a",64)+`","size":256}`))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	a.handleDirectUploadAuth(w,r)
	if w.Code != http.StatusServiceUnavailable { t.Fatalf("must require two independent relays: %d", w.Code) }
}
