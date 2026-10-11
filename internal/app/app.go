package app

import (
	"context"
	"net/http"

	"anonblossom/internal/blossom"
	"anonblossom/internal/config"
	"anonblossom/internal/guard"
	"anonblossom/internal/web"
)

// Server contains the HTTP application and its storage dependencies.
type Server struct {
	cfg             config.Config
	storage         *blossom.Client
	pages           *web.Pages
	uploads         chan struct{}
	rlncUploads     chan struct{}
	directAuthSlots chan struct{}
	downloads       chan struct{}
	heavyDownloads  chan struct{}
	cancelDiscovery context.CancelFunc
	http            *http.Server
	admission       *guard.Guard
	cache           *downloadCache
}
