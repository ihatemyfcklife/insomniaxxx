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
	DefaultListenAddr          = ":8080"
	DefaultChunkSize           = 1 << 20
	DefaultMaxUploadBytes      = 19 << 20
	DefaultUploadConcurrency   = 8
	MaxConfigurableUploadBytes = 64 << 20
	MaxUploadConcurrency       = 32
	DefaultDownloadConcurrency = 64
	MaxDownloadConcurrency     = 256
	DefaultHeavyDownloads      = 4
	MaxHeavyDownloads          = 8
	DefaultRLNCUploads         = 1
	MaxRLNCUploads             = 4
	DefaultSpoolBudgetBytes    = 1 << 30
	MaxSpoolBudgetBytes        = 16 << 30
	DefaultBlossomURLs         = "https://cdn.hzrd149.com,https://nostr.download,https://blossom.poster.place,https://cdn.sovbit.host"
)

// Config contains runtime settings loaded from environment variables.
type Config struct {
	ListenAddr        string
	BlossomURLs       []string
	PublicBaseURL     string
	MaxUploadBytes    int64
	ChunkSize         int
	UploadConcurrency int
	DownloadConcurrency int
	HeavyDownloads      int
	RLNCUploads          int
	SpoolBudgetBytes     int64
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
		DownloadConcurrency: getenvInt("DOWNLOAD_CONCURRENCY", DefaultDownloadConcurrency),
		HeavyDownloads: getenvInt("HEAVY_DOWNLOAD_CONCURRENCY", DefaultHeavyDownloads),
		RLNCUploads: getenvInt("RLNC_UPLOAD_CONCURRENCY", DefaultRLNCUploads),
		SpoolBudgetBytes: int64(getenvInt("SPOOL_BUDGET_BYTES", DefaultSpoolBudgetBytes)),
		OnionGuardSecure:  parseBool("ONIONGUARD_COOKIE_SECURE", false),
	}

	if err := cfg.validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) validate() error {
	if strings.EqualFold(strings.TrimSpace(os.Getenv("APP_ENV")), "production") && c.PublicBaseURL == "" {
		return errors.New("PUBLIC_BASE_URL is mandatory when APP_ENV=production")
	}
	if len(c.BlossomURLs) == 0 {
		return errors.New("BLOSSOM_URLS must contain at least one URL")
	}
	if strings.EqualFold(strings.TrimSpace(os.Getenv("APP_ENV")), "production") && len(c.BlossomURLs) < 2 {
		return errors.New("production requires at least two distinct configured Blossom relays")
	}
	for _, raw := range c.BlossomURLs {
		u, err := url.Parse(raw)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return fmt.Errorf("invalid Blossom URL: %q", raw)
		}
	}
	if c.PublicBaseURL != "" {
		publicURL, err := url.Parse(c.PublicBaseURL)
		if err != nil || (publicURL.Scheme != "https" && publicURL.Scheme != "http") ||
			publicURL.Host == "" || publicURL.Path != "" || publicURL.User != nil || publicURL.RawQuery != "" || publicURL.Fragment != "" {
			return errors.New("PUBLIC_BASE_URL must be an http(s) origin without credentials, query, or fragment")
		}
		if strings.EqualFold(strings.TrimSpace(os.Getenv("APP_ENV")), "production") &&
			publicURL.Scheme == "http" && !strings.HasSuffix(strings.ToLower(publicURL.Hostname()), ".onion") {
			return errors.New("production requires HTTPS except for native HTTP .onion origins")
		}
		if publicURL.Scheme == "https" && !c.OnionGuardSecure {
			return errors.New("ONIONGUARD_COOKIE_SECURE must be true when PUBLIC_BASE_URL uses HTTPS")
		}
	}
	if c.MaxUploadBytes <= 0 || c.MaxUploadBytes > MaxConfigurableUploadBytes {
		return fmt.Errorf("MAX_UPLOAD_BYTES must be between 1 and %d bytes (64 MiB)", MaxConfigurableUploadBytes)
	}
	if c.ChunkSize < 64<<10 || c.ChunkSize > 8<<20 {
		return errors.New("CHUNK_SIZE must be between 64 KiB and 8 MiB")
	}
	if c.UploadConcurrency <= 0 || c.UploadConcurrency > MaxUploadConcurrency {
		return fmt.Errorf("UPLOAD_CONCURRENCY must be between 1 and %d", MaxUploadConcurrency)
	}
	if c.DownloadConcurrency < 0 || c.DownloadConcurrency > MaxDownloadConcurrency {
		return fmt.Errorf("DOWNLOAD_CONCURRENCY must be between 1 and %d", MaxDownloadConcurrency)
	}
	if c.HeavyDownloads < 0 || c.HeavyDownloads > MaxHeavyDownloads {
		return fmt.Errorf("HEAVY_DOWNLOAD_CONCURRENCY must be between 1 and %d", MaxHeavyDownloads)
	}
	if c.RLNCUploads < 0 || c.RLNCUploads > MaxRLNCUploads {
		return fmt.Errorf("RLNC_UPLOAD_CONCURRENCY must be between 1 and %d", MaxRLNCUploads)
	}
	if c.SpoolBudgetBytes < 0 || c.SpoolBudgetBytes > MaxSpoolBudgetBytes {
		return fmt.Errorf("SPOOL_BUDGET_BYTES must be between %d and %d", c.MaxUploadBytes+(4<<20), int64(MaxSpoolBudgetBytes))
	}
	if c.SpoolBudgetBytes > 0 && c.SpoolBudgetBytes < c.MaxUploadBytes+(4<<20) {
		return errors.New("SPOOL_BUDGET_BYTES must hold at least one maximum-size encrypted blob")
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
