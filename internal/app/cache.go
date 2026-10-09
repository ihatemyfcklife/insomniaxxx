package app

import (
	"context"
	"os"
	"sync"
	"time"

	"anonblossom/internal/cryptbox"
)

type cacheEntry struct {
	path      string
	meta      cryptbox.FileMeta
	expiresAt time.Time
	ready     chan struct{}
	err       error
}

type downloadCache struct {
	mu      sync.Mutex
	entries map[string]*cacheEntry
	ttl     time.Duration
	stop    chan struct{}
}

func newDownloadCache(ttl time.Duration) *downloadCache {
	c := &downloadCache{
		entries: make(map[string]*cacheEntry),
		ttl:     ttl,
		stop:    make(chan struct{}),
	}
	go c.cleaner()
	return c
}

func (c *downloadCache) Close() {
	close(c.stop)
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, entry := range c.entries {
		if entry.path != "" {
			_ = os.Remove(entry.path)
		}
	}
	c.entries = make(map[string]*cacheEntry)
}

func (c *downloadCache) cleaner() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.stop:
			return
		case now := <-ticker.C:
			c.mu.Lock()
			for key, entry := range c.entries {
				if now.After(entry.expiresAt) {
					delete(c.entries, key)
					if entry.path != "" {
						_ = os.Remove(entry.path)
					}
				}
			}
			c.mu.Unlock()
		}
	}
}

// GetOrDecrypt renvoie un handle de lecture vers le fichier déchiffré.
// Si plusieurs requêtes demandent le même fichier en même temps, une seule effectue
// le déchiffrement ; les autres attendent la fin de l'opération.
func (c *downloadCache) GetOrDecrypt(
	ctx context.Context,
	key string,
	decryptFn func() (string, cryptbox.FileMeta, error),
) (*os.File, cryptbox.FileMeta, error) {
	c.mu.Lock()
	entry, exists := c.entries[key]
	if exists {
		entry.expiresAt = time.Now().Add(c.ttl)
		c.mu.Unlock()

		select {
		case <-entry.ready:
		case <-ctx.Done():
			return nil, cryptbox.FileMeta{}, ctx.Err()
		}

		if entry.err != nil {
			return nil, cryptbox.FileMeta{}, entry.err
		}

		f, err := os.Open(entry.path)
		if err != nil {
			c.mu.Lock()
			delete(c.entries, key)
			c.mu.Unlock()
			return nil, cryptbox.FileMeta{}, err
		}
		return f, entry.meta, nil
	}

	entry = &cacheEntry{
		expiresAt: time.Now().Add(c.ttl),
		ready:     make(chan struct{}),
	}
	c.entries[key] = entry
	c.mu.Unlock()

	path, meta, err := decryptFn()
	if err != nil {
		entry.err = err
		close(entry.ready)
		c.mu.Lock()
		delete(c.entries, key)
		c.mu.Unlock()
		return nil, cryptbox.FileMeta{}, err
	}

	entry.path = path
	entry.meta = meta
	close(entry.ready)

	f, err := os.Open(entry.path)
	if err != nil {
		return nil, cryptbox.FileMeta{}, err
	}
	return f, entry.meta, nil
}
