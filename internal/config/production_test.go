package config

import "testing"

func TestProductionRequiresCanonicalOrigin(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	cfg := Config{
		ListenAddr:     ":8080",
		BlossomURLs:    []string{"https://blossom.example", "https://backup.example"},
		MaxUploadBytes: 19 << 20, ChunkSize: 1 << 20, UploadConcurrency: 2,
	}
	if err := cfg.validate(); err == nil {
		t.Fatal("production without PUBLIC_BASE_URL accepted")
	}
	cfg.PublicBaseURL = "https://files.example"
	cfg.OnionGuardSecure = true
	if err := cfg.validate(); err != nil {
		t.Fatalf("valid production config rejected: %v", err)
	}
}
