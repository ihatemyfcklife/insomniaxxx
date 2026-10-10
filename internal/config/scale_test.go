package config

import "testing"

func TestScaleEnvironmentLoadsAndValidates(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	t.Setenv("PUBLIC_BASE_URL", "https://files.example")
	t.Setenv("ONIONGUARD_COOKIE_SECURE", "true")
	t.Setenv("BLOSSOM_URLS", "https://blossom.example,https://backup.example")
	t.Setenv("UPLOAD_CONCURRENCY", "16")
	t.Setenv("DOWNLOAD_CONCURRENCY", "128")
	t.Setenv("HEAVY_DOWNLOAD_CONCURRENCY", "8")
	t.Setenv("RLNC_UPLOAD_CONCURRENCY", "3")
	t.Setenv("SPOOL_BUDGET_BYTES", "2147483648")
	t.Setenv("ONIONGUARD_REDIS_ADDR", "redis.private:6379")
	t.Setenv("ONIONGUARD_REDIS_PASSWORD", "example-only")
	cfg, err := Load()
	if err != nil { t.Fatal(err) }
	if cfg.UploadConcurrency != 16 || cfg.DownloadConcurrency != 128 || cfg.HeavyDownloads != 8 || cfg.RLNCUploads != 3 || cfg.SpoolBudgetBytes != 2147483648 || cfg.GuardRedisAddr != "redis.private:6379" || cfg.GuardRedisPassword != "example-only" {
		t.Fatalf("unexpected scaling configuration: %+v", cfg)
	}
}

func TestScaleLimitsRejectInvalidSettings(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	cfg := Config{ListenAddr:":8080",BlossomURLs:[]string{"https://one.example","https://two.example"},MaxUploadBytes:19<<20,ChunkSize:1<<20,UploadConcurrency:8}
	cases := []struct {
		name string
		change func(*Config)
	}{
		{"download limit", func(c *Config){c.DownloadConcurrency=MaxDownloadConcurrency+1}},
		{"heavy limit", func(c *Config){c.HeavyDownloads=MaxHeavyDownloads+1}},
		{"rlnc limit", func(c *Config){c.RLNCUploads=MaxRLNCUploads+1}},
		{"spool too small", func(c *Config){c.SpoolBudgetBytes=1}},
		{"spool too large", func(c *Config){c.SpoolBudgetBytes=MaxSpoolBudgetBytes+1}},
		{"redis password without host", func(c *Config){c.GuardRedisPassword="secret"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			candidate:=cfg
			tc.change(&candidate)
			if err:=candidate.validate();err==nil {t.Fatal("expected rejection")}
		})
	}
}
