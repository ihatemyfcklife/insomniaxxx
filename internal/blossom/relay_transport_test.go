package blossom

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestRelayRequestsDoNotFollowRedirects(t *testing.T) {
	var targetHit atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetHit.Store(true)
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()

	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer redirector.Close()

	client := NewClientWithOptions([]string{redirector.URL}, http.DefaultClient, "test-agent", ClientOptions{AllowPrivateRelays: true})
	if _, err := client.Fetch(context.Background(), "0123456789abcdef"); err == nil {
		t.Fatal("expected non-200 relay response to fail")
	}
	if targetHit.Load() {
		t.Fatal("HTTP client followed a relay redirect")
	}
}
