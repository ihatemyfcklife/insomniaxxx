package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"anonblossom/internal/blossom"
	"anonblossom/internal/config"
	"anonblossom/internal/guard"
	"anonblossom/internal/web"
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
		MaxIdleConns:          64,
		IdleConnTimeout:       90 * time.Second,
	}

	guardConfig := guard.Configure(cfg.MaxUploadBytes+4<<20, cfg.OnionGuardSecure)
	admission, err := guard.New(guardConfig)
	if err != nil {
		return nil, err
	}

	cache := newDownloadCache(3 * time.Minute)

	// User-Agent transparent avec fallback sur l'instance ou le projet
	userAgent := "anonblossom/1.0 (+https://files.example.com)"
	if cfg.PublicBaseURL != "" {
		userAgent = fmt.Sprintf("anonblossom/1.0 (+%s)", cfg.PublicBaseURL)
	}

	storageClient := blossom.NewClient(cfg.BlossomURLs, &http.Client{Transport: transport}, userAgent)

	a := &Server{
		cfg:       cfg,
		pages:     pages,
		admission: admission,
		cache:     cache,
		storage:   storageClient,
		uploads:   make(chan struct{}, cfg.UploadConcurrency),
	}

	a.http = &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           a.routes(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       90 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	// Lancement de la découverte toutes les 30 minutes
	storageClient.StartDiscovery(context.Background(), 30*time.Minute)

	return a, nil
}

func (a *Server) Run() error {
	log.Printf("listening on %s", a.cfg.ListenAddr)
	log.Printf("blossom initial seed instances: %s", strings.Join(a.cfg.BlossomURLs, ", "))

	if err := a.http.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func (a *Server) Close() {
	if a == nil {
		return
	}
	if a.cache != nil {
		a.cache.Close()
	}
	if a.admission != nil {
		a.admission.Close()
	}
}
