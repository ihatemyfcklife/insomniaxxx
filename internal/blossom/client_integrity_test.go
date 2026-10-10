package blossom

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestFetchRejectsCorruptedRelayAndUsesHealthyReplica(t *testing.T) {
	blob := []byte("authenticated ciphertext fixture")
	digest := sha256.Sum256(blob)
	hash := hex.EncodeToString(digest[:])
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("corrupt"))
	}))
	defer bad.Close()
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(blob)
	}))
	defer good.Close()
	client := NewClientWithOptions([]string{bad.URL, good.URL}, http.DefaultClient, "test", ClientOptions{AllowPrivateRelays: true})
	resp, err := client.Fetch(context.Background(), hash)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	got, err := io.ReadAll(resp.Body)
	if err != nil || string(got) != string(blob) {
		t.Fatalf("expected valid replica, got %q, %v", got, err)
	}
	if client.safeHTTP.Timeout != 2*time.Minute {
		t.Fatal("untrusted relay requests must have a bounded lifetime")
	}
}

func TestRelayURLRejectsQueryAndPath(t *testing.T) {
	for _, value := range []string{"https://8.8.8.8/api", "https://8.8.8.8?x=y", "https://8.8.8.8/#abc"} {
		if err := publicRelayURL(context.Background(), value); err == nil {
			t.Fatalf("accepted non-origin relay URL %q", value)
		}
	}
}
