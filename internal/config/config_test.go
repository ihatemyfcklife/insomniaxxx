package config

import (
	"strings"
	"testing"
)

func TestParseURLs(t *testing.T) {
	got := parseURLs(" https://a.example/,https://b.example,https://a.example/ ,")
	if len(got) != 2 {
		t.Fatalf("expected 2 URLs, got %#v", got)
	}
	if got[0] != "https://a.example" || got[1] != "https://b.example" {
		t.Fatalf("unexpected URL normalization: %#v", got)
	}
}

func TestConfigValidation(t *testing.T) {
	cfg := Config{
		ListenAddr:        ":8080",
		BlossomURLs:       []string{"https://blossom.example"},
		MaxUploadBytes:    1 << 20,
		ChunkSize:         1 << 20,
		UploadConcurrency: 1,
	}
	if err := cfg.validate(); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}

	cfg.ChunkSize = 32 << 10
	if err := cfg.validate(); err == nil {
		t.Fatal("invalid chunk size accepted")
	}
}

func TestDefaultBlossomURLsAreGenericServers(t *testing.T) {
	for _, host := range []string{
		"cdn.hzrd149.com",
		"nostr.download",
		"blossom.poster.place",
		"cdn.sovbit.host",
	} {
		if !strings.Contains(DefaultBlossomURLs, host) {
			t.Fatalf("default Blossom servers missing %q", host)
		}
	}
	if strings.Contains(DefaultBlossomURLs, "nostr.build") {
		t.Fatal("media-only nostr.build endpoint must not be a default server")
	}
}
