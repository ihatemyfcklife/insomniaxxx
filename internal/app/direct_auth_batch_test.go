package app

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDirectUploadBatchScopesEachHashAndBoundsCPU(t *testing.T) {
	a := directTestServer()
	hash := func(i byte) string { return strings.Repeat(string(i), 64) }
	blobs := make([]directBlobRequest, 8)
	for i := range blobs {
		blobs[i] = directBlobRequest{Hash: hash(byte('a' + i)), Size: 32}
	}
	send := func(body []byte, origin string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/direct-upload-auth/batch", strings.NewReader(string(body)))
		r.Header.Set("Content-Type", "application/json")
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		w := httptest.NewRecorder()
		a.handleDirectUploadAuthBatch(w, r)
		return w
	}
	payload, _ := json.Marshal(map[string]any{"blobs": blobs})
	w := send(payload, "https://files.example")
	if w.Code != http.StatusOK {
		t.Fatalf("batch rejected: %d %s", w.Code, w.Body.String())
	}
	var response struct {
		Grants []directBatchGrant `json:"grants"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Grants) != 8 {
		t.Fatalf("expected 8 signed blobs, got %d", len(response.Grants))
	}
	for i, grant := range response.Grants {
		if grant.Hash != blobs[i].Hash || len(grant.Targets) != 3 {
			t.Fatalf("incorrect per-blob relay permissions: %+v", grant)
		}
		for _, target := range grant.Targets {
			raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(target.Authorization, "Nostr "))
			if err != nil { t.Fatal(err) }
			var event struct {
				Kind int `json:"kind"`
				Tags [][]string `json:"tags"`
			}
			if err := json.Unmarshal(raw, &event); err != nil { t.Fatal(err) }
			if event.Kind != 24242 { t.Fatal("invalid Nostr upload event") }
			found := false
			for _, tag := range event.Tags {
				if len(tag) > 1 && tag[0] == "x" && tag[1] == grant.Hash {
					found = true
				}
			}
			if !found { t.Fatal("batch granted unscoped relay upload") }
		}
	}
	invalid := []struct {
		label string
		data []byte
		origin string
	}{
		{"foreign origin",payload,"https://evil.example"},
		{"missing list",[]byte(`{}`),""},
		{"excess entries",mustBatchJSON(t, append(blobs, blobs[0])),""},
		{"duplicate hashes",mustBatchJSON(t, []directBlobRequest{blobs[0], blobs[0]}),""},
		{"invalid size",mustBatchJSON(t, []directBlobRequest{{Hash:hash('e'),Size:0}}),""},
		{"invalid hash",mustBatchJSON(t, []directBlobRequest{{Hash:"bad", Size:32}}),""},
		{"unknown property",[]byte(`{"blobs":[{"hash":"`+hash('e')+`","size":32,"url":"https://evil.example"}]}`),""},
		{"trailing body",append(append([]byte{},payload...),[]byte(` {}`)...),""},
		{"huge body",[]byte(strings.Repeat(" ",2049)),""},
	}
	for _, tc := range invalid {
		t.Run(tc.label, func(t *testing.T) {
			if got := send(tc.data,tc.origin); got.Code == http.StatusOK {
				t.Fatalf("accepted invalid authorization batch: %s",got.Body.String())
			}
		})
	}
}

func mustBatchJSON(t *testing.T, blobs []directBlobRequest) []byte {
	t.Helper()
	b, err := json.Marshal(struct{Blobs []directBlobRequest `json:"blobs"`}{blobs})
	if err != nil { t.Fatal(err) }
	return b
}

func TestBatchSigningCapacityFailsClosed(t *testing.T) {
	a := directTestServer()
	a.directAuthSlots = make(chan struct{}, 1)
	a.directAuthSlots <- struct{}{}
	body := `{"blobs":[{"hash":"`+strings.Repeat("a",64)+`","size":32}]}`
	r := httptest.NewRequest("POST", "/direct-upload-auth/batch", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	a.handleDirectUploadAuthBatch(w, r)
	if w.Code != http.StatusServiceUnavailable || w.Header().Get("Retry-After") != "1" {
		t.Fatalf("saturated signer did not fail closed: %d %s",w.Code,w.Body.String())
	}
}
