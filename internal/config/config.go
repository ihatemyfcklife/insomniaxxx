package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
)

const (
	DefaultListenAddr        = ":8080"
	DefaultChunkSize         = 1 << 20
	DefaultMaxUploadBytes    = 19 << 20
	DefaultUploadConcurrency = 2
	DefaultBlossomURLs       = "https://cdn.hzrd149.com,https://nostr.download,https://blossom.poster.place,https://cdn.sovbit.host"
)

// Config contains runtime settings loaded from environment variables.
type Config struct {
	ListenAddr        string
	BlossomURLs       []string
	PublicBaseURL     string
	MaxUploadBytes    int64
	ChunkSize         int
	UploadConcurrency int
	OnionGuardSecure  bool
}

// Load reads and validates application settings from the environment.
func Load() (Config, error) {
	cfg := Config{
		ListenAddr:        getenv("LISTEN_ADDR", DefaultListenAddr),
		BlossomURLs:       parseURLs(getenv("BLOSSOM_URLS", DefaultBlossomURLs)),
		PublicBaseURL:     strings.TrimRight(os.Getenv("PUBLIC_BASE_URL"), "/"),
		MaxUploadBytes:    int64(getenvInt("MAX_UPLOAD_BYTES", DefaultMaxUploadBytes)),
		ChunkSize:         getenvInt("CHUNK_SIZE", DefaultChunkSize),
		UploadConcurrency: getenvInt("UPLOAD_CONCURRENCY", DefaultUploadConcurrency),
		OnionGuardSecure:  parseBool("ONIONGUARD_COOKIE_SECURE", false),
	}

	if err := cfg.validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) validate() error {
	if len(c.BlossomURLs) == 0 {
		return errors.New("BLOSSOM_URLS must contain at least one URL")
	}
	for _, raw := range c.BlossomURLs {
		u, err := url.Parse(raw)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return fmt.Errorf("invalid Blossom URL: %q", raw)
		}
	}
	if c.MaxUploadBytes <= 0 {
		return errors.New("MAX_UPLOAD_BYTES must be positive")
	}
	if c.ChunkSize < 64<<10 || c.ChunkSize > 8<<20 {
		return errors.New("CHUNK_SIZE must be between 64 KiB and 8 MiB")
	}
	if c.UploadConcurrency <= 0 {
		return errors.New("UPLOAD_CONCURRENCY must be positive")
	}
	return nil
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func getenvInt(key string, fallback int) int {
	value, err := strconv.Atoi(os.Getenv(key))
	if err != nil || value == 0 {
		return fallback
	}
	return value
}

func parseBool(key string, fallback bool) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func parseURLs(raw string) []string {
	seen := make(map[string]struct{})
	urls := make([]string, 0, 4)

	for _, part := range strings.Split(raw, ",") {
		value := strings.TrimRight(strings.TrimSpace(part), "/")
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		urls = append(urls, value)
	}
	return urls
}
