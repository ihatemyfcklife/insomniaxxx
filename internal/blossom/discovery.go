package blossom

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"fiatjaf.com/nostr"
)

var defaultNostrRelays = []string{
	"wss://relay.damus.io",
	"wss://nos.lol",
	"wss://relay.nostr.band",
	"wss://relay.primal.net",
}

// discoverFromNostr interroge les relais Nostr pour récupérer les serveurs Blossom (BUD-04).
func discoverFromNostr(ctx context.Context) []string {
	found := make(map[string]struct{})
	filter := nostr.Filter{
		Kinds: []nostr.Kind{10063},
		Limit: 30,
	}

	for _, relayURL := range defaultNostrRelays {
		reqCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
		relay, err := nostr.RelayConnect(reqCtx, relayURL, nostr.RelayOptions{})
		if err != nil {
			cancel()
			continue
		}

		for ev := range relay.QueryEvents(filter) {
			for _, tag := range ev.Tags {
				if len(tag) >= 2 && tag[0] == "server" {
					if clean, ok := cleanBlossomURL(tag[1]); ok {
						found[clean] = struct{}{}
					}
				}
			}
		}
		relay.Close()
		cancel()
	}

	urls := make([]string, 0, len(found))
	for u := range found {
		urls = append(urls, u)
	}
	return urls
}

// cleanBlossomURL valide et nettoie l'URL tout en prévenant les attaques SSRF.
func cleanBlossomURL(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimRight(raw, "/")
	u, err := url.Parse(raw)
	if err != nil {
		return "", false
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return "", false
	}
	if u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", false
	}

	host := strings.ToLower(u.Hostname())
	// Blocage SSRF (localhost, IP privées et domaines internes)
	if host == "localhost" || host == "127.0.0.1" || host == "::1" ||
		strings.HasSuffix(host, ".local") || strings.HasSuffix(host, ".internal") ||
		strings.HasPrefix(host, "10.") || strings.HasPrefix(host, "192.168.") ||
		(strings.HasPrefix(host, "172.") && len(host) >= 7) {
		return "", false
	}

	return fmt.Sprintf("%s://%s", u.Scheme, u.Host), true
}

// checkServerHealth teste la disponibilité et mesure la latence d'une instance Blossom.
func (c *Client) checkServerHealth(ctx context.Context, status *ServerStatus) {
	reqCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	start := time.Now()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, status.URL, nil)
	if err != nil {
		c.recordHealth(status, 0, false)
		return
	}
	req.Header.Set("User-Agent", c.UserAgent)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		c.recordHealth(status, 0, false)
		return
	}
	defer resp.Body.Close()
	_, _ = io.CopyN(io.Discard, resp.Body, 512)

	c.recordHealth(status, time.Since(start), resp.StatusCode < 500)
}

// recordHealth applique le résultat de santé sous le verrou du client pour
// éviter les données concurrentes avec la page /relays.
func (c *Client) recordHealth(status *ServerStatus, latency time.Duration, online bool) {
	c.mu.Lock()
	status.Online = online
	status.Latency = latency
	status.LastChecked = time.Now()
	c.mu.Unlock()
}
