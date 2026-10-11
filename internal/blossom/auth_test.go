package blossom

import (
	"encoding/base64"
	"encoding/json"
	"testing"

	"fiatjaf.com/nostr"
)

func TestAuthorizationScopesUploadToBlob(t *testing.T) {
	const hash = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

	token, err := authorization("https://blossom.example", hash)
	if err != nil {
		t.Fatal(err)
	}

	const prefix = "Nostr "
	if len(token) <= len(prefix) || token[:len(prefix)] != prefix {
		t.Fatalf("unexpected authorization prefix: %q", token)
	}

	raw, err := base64.StdEncoding.DecodeString(token[len(prefix):])
	if err != nil {
		t.Fatal(err)
	}

	// Verify compatibility with the independent full Nostr event parser and
	// verifier used on the native server, not only our signing wire struct.
	var signed nostr.Event
	if err := json.Unmarshal(raw, &signed); err != nil {
		t.Fatal(err)
	}
	if !signed.CheckID() || !signed.VerifySignature() {
		t.Fatal("local upload authorization is not a valid Nostr event")
	}
	signed.Content += "tampered"
	if signed.VerifySignature() {
		t.Fatal("tampered upload authorization was accepted")
	}

	var event struct {
		Kind int        `json:"kind"`
		Tags [][]string `json:"tags"`
	}
	if err := json.Unmarshal(raw, &event); err != nil {
		t.Fatal(err)
	}
	if event.Kind != 24242 {
		t.Fatalf("kind = %d, want 24242", event.Kind)
	}

	var foundUpload, foundHash bool
	for _, tag := range event.Tags {
		if len(tag) < 2 {
			continue
		}
		switch tag[0] {
		case "t":
			foundUpload = tag[1] == "upload"
		case "x":
			foundHash = tag[1] == hash
		}
	}
	if !foundUpload || !foundHash {
		t.Fatalf("authorization missing upload/x scope: %#v", event.Tags)
	}
}
