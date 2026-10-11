package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"anonblossom/internal/blossom"
	"anonblossom/internal/config"
	"anonblossom/internal/guard"
	"anonblossom/internal/web"
	onionguard "github.com/ihatemyfcklife/onionguard"
)

func NewServer(cfg config.Config) (*Server, error) {
	pages, err := web.NewPages()
	if err != nil {
		return nil, err
	}

	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout: 10 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		MaxIdleConns:          512,
		MaxIdleConnsPerHost:   64,
		MaxConnsPerHost:       128,
		IdleConnTimeout:       90 * time.Second,
	}

	guardConfig := guard.Configure(cfg.MaxUploadBytes+4<<20, cfg.OnionGuardSecure)
	// A shared OnionGuard store is required when load-balancing multiple
	// application replicas: challenges, sessions and rate limits must agree.
	if cfg.GuardRedisAddr != "" {
		guardConfig.StoreConfig.Type = onionguard.StoreTypeRedis
		guardConfig.StoreConfig.RedisAddr = cfg.GuardRedisAddr
		guardConfig.StoreConfig.RedisPassword = cfg.GuardRedisPassword
		guardConfig.StoreConfig.RedisFailClosed = true
	}
	admission, err := guard.New(guardConfig)
	if err != nil {
		return nil, err
	}

	cache := newDownloadCache(3 * time.Minute)
	discoveryCtx, cancelDiscovery := context.WithCancel(context.Background())

	// User-Agent transparent avec fallback sur l'instance ou le projet
	userAgent := "anonblossom/1.0 (+https://files.example.com)"
	if cfg.PublicBaseURL != "" {
		userAgent = fmt.Sprintf("anonblossom/1.0 (+%s)", cfg.PublicBaseURL)
	}

	storageClient := blossom.NewClientWithOptions(cfg.BlossomURLs, &http.Client{Transport: transport}, userAgent,
		blossom.ClientOptions{MaxBlobBytes: cfg.MaxUploadBytes + (4 << 20)})

	// Bound disk spooling by the maximum encrypted blob size, not just by
	// the number of active ciphertext downloads. This prevents large files
	// from exhausting the temporary filesystem when concurrency increases.
	if cfg.DownloadConcurrency <= 0 {
		cfg.DownloadConcurrency = config.DefaultDownloadConcurrency
	}
	if cfg.HeavyDownloads <= 0 {
		cfg.HeavyDownloads = config.DefaultHeavyDownloads
	}
	if cfg.RLNCUploads <= 0 {
		cfg.RLNCUploads = config.DefaultRLNCUploads
	}
	if cfg.SpoolBudgetBytes <= 0 {
		cfg.SpoolBudgetBytes = config.DefaultSpoolBudgetBytes
	}
	blobBudget := cfg.MaxUploadBytes + (4 << 20)
	cipherSlots := cfg.DownloadConcurrency
	if blobBudget > 0 && int64(cipherSlots) > cfg.SpoolBudgetBytes/blobBudget {
		cipherSlots = int(cfg.SpoolBudgetBytes / blobBudget)
	}
	if cipherSlots < 1 {
		cipherSlots = 1
	}

	a := &Server{
		cfg:             cfg,
		pages:           pages,
		admission:       admission,
		cache:           cache,
		storage:         storageClient,
		uploads:         make(chan struct{}, cfg.UploadConcurrency),
		rlncUploads:     make(chan struct{}, cfg.RLNCUploads),
		directAuthSlots: make(chan struct{}, 4),
		downloads:       make(chan struct{}, cipherSlots),
		heavyDownloads:  make(chan struct{}, cfg.HeavyDownloads),
		cancelDiscovery: cancelDiscovery,
	}

	a.http = &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           a.routes(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       5 * time.Minute,
		WriteTimeout:      10 * time.Minute,
		IdleTimeout:       90 * time.Second,
		MaxHeaderBytes:    32 << 10,
	}

	// Lancement de la découverte toutes les 30 minutes
	storageClient.StartDiscovery(discoveryCtx, 30*time.Minute)

	return a, nil
}

func (a *Server) Run() error {
	log.Printf("listening on %s", a.cfg.ListenAddr)
	log.Printf("blossom initial seed instances: %s", strings.Join(a.cfg.BlossomURLs, ", "))

	signals, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- a.http.ListenAndServe()
	}()

	select {
	case err := <-serveErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	case <-signals.Done():
		if a.cancelDiscovery != nil {
			a.cancelDiscovery()
		}
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := a.http.Shutdown(shutdownCtx); err != nil {
			_ = a.http.Close()
			return err
		}
		err := <-serveErr
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	}
}

func (a *Server) Close() {
	if a == nil {
		return
	}
	if a.cancelDiscovery != nil {
		a.cancelDiscovery()
	}
	if a.cache != nil {
		a.cache.Close()
	}
	if a.admission != nil {
		a.admission.Close()
	}
}
