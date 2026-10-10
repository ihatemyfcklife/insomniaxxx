package blossom

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// FetchVerifiedFile reads an untrusted relay response to bounded temporary
// storage and verifies its complete SHA-256 BEFORE returning bytes to a client.
// The caller owns the returned file and MUST close and unlink it.
func (c *Client) FetchVerifiedFile(ctx context.Context, hash string) (*os.File, error) {
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	if len(hash) != sha256.Size*2 {
		return nil, errors.New("invalid blob hash")
	}
	if _, err := hex.DecodeString(hash); err != nil {
		return nil, errors.New("invalid blob hash")
	}

	var lastErr error
	for _, baseURL := range c.GetActiveURLs() {
		if err := relayURLAllowed(ctx, baseURL, c.allowPrivateRelays); err != nil {
			lastErr = err
			continue
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(baseURL, "/")+"/"+hash, nil)
		if err != nil {
			lastErr = err
			continue
		}
		req.Header.Set("User-Agent", c.UserAgent)
		req.Header.Set("Accept", "application/octet-stream")
		resp, err := c.safeHTTP.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode != http.StatusOK {
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
			_ = resp.Body.Close()
			if resp.StatusCode != http.StatusNotFound {
				lastErr = fmt.Errorf("relay returned HTTP %d", resp.StatusCode)
			}
			continue
		}
		// An encrypted blob may be 64 MiB or larger with framing overhead.
		// Never accumulate it into a giant per-request heap buffer.
		tmp, err := os.CreateTemp("", "anonblossom-verified-*")
		if err != nil {
			_ = resp.Body.Close()
			return nil, err
		}
		digest := sha256.New()
		n, readErr := io.Copy(io.MultiWriter(tmp, digest), io.LimitReader(resp.Body, c.maxBlobBytes+1))
		_ = resp.Body.Close()
		if readErr == nil && n > c.maxBlobBytes {
			readErr = errors.New("relay blob exceeds size limit")
		}
		if readErr == nil && !strings.EqualFold(hex.EncodeToString(digest.Sum(nil)), hash) {
			readErr = errors.New("relay blob SHA-256 mismatch")
		}
		if readErr == nil {
			_, readErr = tmp.Seek(0, io.SeekStart)
		}
		if readErr == nil {
			return tmp, nil
		}
		lastErr = readErr
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
	}
	if lastErr == nil {
		return nil, ErrNotFound
	}
	return nil, fmt.Errorf("all Blossom relays failed verification: %w", lastErr)
}
