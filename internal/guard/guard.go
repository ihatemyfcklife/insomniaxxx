package guard

import (
	"net/http"

	onionguard "github.com/ihatemyfcklife/onionguard"
	onionmw "github.com/ihatemyfcklife/onionguard/middleware"
)

// Guard adapts OnionGuard to the standard net/http middleware shape used by
// the application.
type Guard struct {
	engine     *onionguard.Engine
	middleware func(http.Handler) http.Handler
}

// New creates an OnionGuard engine and prepares its HTTP middleware.
func New(cfg onionguard.Config) (*Guard, error) {
	engine, err := onionguard.New(cfg)
	if err != nil {
		return nil, err
	}

	return &Guard{
		engine:     engine,
		middleware: onionmw.Middleware(engine),
	}, nil
}

// Middleware applies OnionGuard before the application handler.
func (g *Guard) Middleware(next http.Handler) http.Handler {
	if g == nil || g.middleware == nil {
		return next
	}
	return g.middleware(next)
}

// Close shuts down the OnionGuard engine.
func (g *Guard) Close() {
	if g == nil || g.engine == nil {
		return
	}
	_ = g.engine.Close()
}
