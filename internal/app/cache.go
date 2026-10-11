package app

import (
	"context"
	"errors"
	"os"
	"sync"
	"time"

	"anonblossom/internal/cryptbox"
)

const (
	maxPlainCacheBytes   int64 = 128 << 20
	maxPlainCacheEntries       = 2
)

type cacheEntry struct {
	size      int64
	path      string
	meta      cryptbox.FileMeta
	expiresAt time.Time
	ready     chan struct{}
	err       error
}

type downloadCache struct {
	mu        sync.Mutex
	entries   map[string]*cacheEntry
	ttl       time.Duration
	stop      chan struct{}
	closeOnce sync.Once
	closed    bool
	usedBytes int64
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
	c.closeOnce.Do(func() {
		close(c.stop)
		c.mu.Lock()
		defer c.mu.Unlock()
		c.closed = true
		for _, entry := range c.entries {
			if entry.path != "" {
				_ = os.Remove(entry.path)
			}
		}
		c.entries = make(map[string]*cacheEntry)
		c.usedBytes = 0
	})
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
					// Never evict an in-flight decrypt. Otherwise its future
					// plaintext file can become orphaned on disk.
					select {
					case <-entry.ready:
					default:
						continue
					}
					delete(c.entries, key)
					c.usedBytes -= entry.size
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
retry:
	if err := ctx.Err(); err != nil {
		return nil, cryptbox.FileMeta{}, err
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, cryptbox.FileMeta{}, errors.New("download cache is closed")
	}
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

		// Serialize opening with eviction: an unlinked cache path must never
		// turn a valid concurrent request into a spurious storage failure.
		c.mu.Lock()
		if c.closed {
			c.mu.Unlock()
			return nil, cryptbox.FileMeta{}, errors.New("download cache is closed")
		}
		if c.entries[key] != entry {
			c.mu.Unlock()
			goto retry
		}
		f, err := os.Open(entry.path)
		if err != nil {
			if c.entries[key] == entry {
				delete(c.entries, key)
				c.usedBytes -= entry.size
			}
			c.mu.Unlock()
			return nil, cryptbox.FileMeta{}, err
		}
		c.mu.Unlock()
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
		c.mu.Lock()
		entry.err = err
		close(entry.ready)
		delete(c.entries, key)
		if entry.path != "" {
			_ = os.Remove(entry.path)
		}
		c.mu.Unlock()
		return nil, cryptbox.FileMeta{}, err
	}

	c.mu.Lock()
	if c.closed {
		_ = os.Remove(path)
		entry.err = errors.New("download cache is closed")
		close(entry.ready)
		c.mu.Unlock()
		return nil, cryptbox.FileMeta{}, entry.err
	}
	info, statErr := os.Stat(path)
	if statErr != nil || info.Size() < 0 || info.Size() > maxPlainCacheBytes {
		_ = os.Remove(path)
		entry.err = errors.New("decrypted file cannot fit in cache budget")
		close(entry.ready)
		delete(c.entries, key)
		c.mu.Unlock()
		return nil, cryptbox.FileMeta{}, entry.err
	}
	// Pending decryptions are bounded by heavy-download admission, not the
	// retention quota. Counting them here rejected valid concurrent misses
	// before any completed entry could be evicted.
	completed := 0
	for _, candidate := range c.entries {
		if candidate.path != "" {
			completed++
		}
	}
	// Evict completed entries only. Never remove an in-progress decrypt.
	for completed >= maxPlainCacheEntries || c.usedBytes+info.Size() > maxPlainCacheBytes {
		var oldestKey string
		var oldest *cacheEntry
		for candidateKey, candidate := range c.entries {
			if candidateKey == key || candidate.path == "" {
				continue
			}
			select {
			case <-candidate.ready:
			default:
				continue
			}
			if oldest == nil || candidate.expiresAt.Before(oldest.expiresAt) {
				oldestKey, oldest = candidateKey, candidate
			}
		}
		if oldest == nil {
			_ = os.Remove(path)
			entry.err = errors.New("decrypted cache capacity exhausted")
			close(entry.ready)
			delete(c.entries, key)
			c.mu.Unlock()
			return nil, cryptbox.FileMeta{}, entry.err
		}
		delete(c.entries, oldestKey)
		completed--
		c.usedBytes -= oldest.size
		_ = os.Remove(oldest.path)
	}
	f, openErr := os.Open(path)
	if openErr != nil {
		_ = os.Remove(path)
		entry.err = openErr
		close(entry.ready)
		delete(c.entries, key)
		c.mu.Unlock()
		return nil, cryptbox.FileMeta{}, openErr
	}
	entry.path, entry.size, entry.meta = path, info.Size(), meta
	c.usedBytes += info.Size()
	entry.expiresAt = time.Now().Add(c.ttl)
	close(entry.ready)
	c.mu.Unlock()
	return f, meta, nil
}

