package app

import (
	"net/http"

	"anonblossom/internal/blossom"
	"anonblossom/internal/config"
	"anonblossom/internal/guard"
	"anonblossom/internal/web"
)

// Server contains the HTTP application and its storage dependencies.
type Server struct {
	cfg       config.Config
	storage   *blossom.Client
	pages     *web.Pages
	uploads   chan struct{}
	http      *http.Server
	admission *guard.Guard
	cache     *downloadCache
}
