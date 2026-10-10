package config

import "testing"

func TestProductionModeFailClosedOnTypo(t *testing.T) {
	cfg := Config{
		ListenAddr:        ":8080",
		BlossomURLs:       []string{"https://blossom.example", "https://replica.example"},
		PublicBaseURL:     "https://files.example",
		MaxUploadBytes:    19 << 20,
		ChunkSize:         1 << 20,
		UploadConcurrency: 2,
		OnionGuardSecure:  true,
	}
	t.Setenv("APP_ENV", "prodution")
	if err := cfg.validate(); err == nil {
		t.Fatal("a misspelled production mode silently bypassed deployment checks")
	}
	t.Setenv("APP_ENV", "production")
	if err := cfg.validate(); err != nil {
		t.Fatalf("valid production config rejected: %v", err)
	}
}

func TestProductionRejectsUnencryptedBlossomOrigins(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	cfg := Config{
		ListenAddr:        ":8080",
		BlossomURLs:       []string{"https://blossom.example", "http://relay.example"},
		PublicBaseURL:     "https://files.example",
		MaxUploadBytes:    19 << 20,
		ChunkSize:         1 << 20,
		UploadConcurrency: 2,
		OnionGuardSecure:  true,
	}
	if err := cfg.validate(); err == nil {
		t.Fatal("unencrypted HTTP Blossom relay accepted in production")
	}
	t.Setenv("APP_ENV", "development")
	if err := cfg.validate(); err != nil {
		t.Fatalf("HTTP staging relay should remain permitted for local development: %v", err)
	}
}
