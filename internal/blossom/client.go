package blossom

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ihatemyfcklife/badrlnc"

	"anonblossom/internal/rlncbox"
)

var ErrNotFound = errors.New("blob not found")

const (
	// maxDiscoveredServers plafonne le nombre de serveurs tiers en mémoire.
	maxDiscoveredServers = 50

	// blacklistDuration est la durée de quarantaine avant réhabilitation.
	blacklistDuration = 6 * time.Hour
)

type ServerStatus struct {
	URL             string
	Host            string
	Online          bool
	Blacklisted     bool
	BlacklistReason string
	BlacklistUntil  time.Time
	Latency         time.Duration
	Source          string // "Config" ou "Discovered"
	LastChecked     time.Time
}

type Client struct {
	mu                 sync.RWMutex
	servers            map[string]*ServerStatus
	activeURLs         []string
	seedURLs           []string
	HTTP               *http.Client
	UserAgent          string
	allowPrivateRelays bool
	safeHTTP           *http.Client
}

// ClientOptions controls optional behavior for isolated environments.
type ClientOptions struct {
	// AllowPrivateRelays is intended only for tests using local httptest servers.
	// Leave false for all production clients.
	AllowPrivateRelays bool
}

type Descriptor struct {
	URL      string `json:"url"`
	SHA256   string `json:"sha256"`
	Size     int64  `json:"size"`
	Type     string `json:"type"`
	Uploaded int64  `json:"uploaded"`
}

func NewClient(seedURLs []string, httpClient *http.Client, userAgent string) *Client {
	return NewClientWithOptions(seedURLs, httpClient, userAgent, ClientOptions{})
}

// NewClientWithOptions creates a client with explicit relay policy options.
func NewClientWithOptions(seedURLs []string, httpClient *http.Client, userAgent string, options ClientOptions) *Client {
	c := &Client{
		servers:            make(map[string]*ServerStatus),
		seedURLs:           seedURLs,
		activeURLs:         seedURLs,
		HTTP:               httpClient,
		UserAgent:          userAgent,
		allowPrivateRelays: options.AllowPrivateRelays,
	}

	for _, u := range seedURLs {
		c.servers[u] = &ServerStatus{
			URL:    u,
			Host:   publicHost(u),
			Source: "Config",
		}
	}
	c.safeHTTP = c.newSafeHTTPClient(httpClient)
	return c
}

// newSafeHTTPClient clones the configured transport but disables proxies and redirects.
// Requests still validate the destination at dial time to prevent DNS rebinding.
func (c *Client) newSafeHTTPClient(base *http.Client) *http.Client {
	if base == nil {
		base = http.DefaultClient
	}
	safeClient := *base
	safeClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	var transport *http.Transport
	if existing, ok := base.Transport.(*http.Transport); ok && existing != nil {
		transport = existing.Clone()
	} else {
		transport = http.DefaultTransport.(*http.Transport).Clone()
	}
	transport.Proxy = nil
	transport.DialContext = c.safeDialContext
	safeClient.Transport = transport
	return &safeClient
}

func (c *Client) safeDialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	var ips []net.IP
	if ip := net.ParseIP(host); ip != nil {
		ips = []net.IP{ip}
	} else {
		answers, lookupErr := net.DefaultResolver.LookupIPAddr(ctx, host)
		if lookupErr != nil || len(answers) == 0 {
			return nil, errors.New("relay hostname did not resolve")
		}
		for _, answer := range answers {
			ips = append(ips, answer.IP)
		}
	}
	if !c.allowPrivateRelays {
		for _, ip := range ips {
			if !isPublicRelayIP(ip) {
				return nil, errors.New("relay resolves to a non-public IP address")
			}
		}
	}
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	var lastErr error
	for _, ip := range ips {
		conn, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if dialErr == nil {
			return conn, nil
		}
		lastErr = dialErr
	}
	if lastErr == nil {
		lastErr = errors.New("relay hostname did not resolve")
	}
	return nil, lastErr
}

var nonPublicRelayCIDRs = func() []*net.IPNet {
	cidrs := []string{
		"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24",
		"192.88.99.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24",
		"224.0.0.0/4", "240.0.0.0/4", "64:ff9b::/96", "64:ff9b:1::/48", "100::/64", "2002::/16",
		"2001::/23", "2001:db8::/32", "5f00::/16",
	}
	nets := make([]*net.IPNet, 0, len(cidrs))
	for _, cidr := range cidrs {
		_, n, err := net.ParseCIDR(cidr)
		if err == nil {
			nets = append(nets, n)
		}
	}
	return nets
}()

func isPublicRelayIP(ip net.IP) bool {
	if ip == nil || !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return false
	}
	for _, network := range nonPublicRelayCIDRs {
		if network.Contains(ip) {
			return false
		}
	}
	return true
}

// GetActiveURLs renvoie les serveurs actuellement en ligne et non blacklistés.
func (c *Client) GetActiveURLs() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if len(c.activeURLs) == 0 {
		return c.seedURLs
	}
	out := make([]string, len(c.activeURLs))
	copy(out, c.activeURLs)
	return out
}

// GetAllServers renvoie la liste complète en réhabilitant à la volée les bans expirés.
func (c *Client) GetAllServers() []ServerStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()

	now := time.Now()
	out := make([]ServerStatus, 0, len(c.servers))
	for _, s := range c.servers {
		item := *s
		if item.Blacklisted && !item.BlacklistUntil.IsZero() && now.After(item.BlacklistUntil) {
			item.Blacklisted = false
			item.BlacklistReason = ""
		}
		out = append(out, item)
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Blacklisted != out[j].Blacklisted {
			return !out[i].Blacklisted
		}
		if out[i].Online != out[j].Online {
			return out[i].Online
		}
		if out[i].Source != out[j].Source {
			return out[i].Source == "Config"
		}
		return out[i].Latency < out[j].Latency
	})
	return out
}

// SelectUploadTargets sélectionne 2 à 3 cibles : 1 serveur de config prioritaire,
// puis 1 ou 2 relais découverts rapides choisis aléatoirement.
func (c *Client) SelectUploadTargets() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	now := time.Now()
	var configHealthy []*ServerStatus
	var discoveredHealthy []*ServerStatus

	for _, s := range c.servers {
		isBlacklisted := s.Blacklisted && (s.BlacklistUntil.IsZero() || now.Before(s.BlacklistUntil))
		if !s.Online || isBlacklisted {
			continue
		}
		if s.Source == "Config" {
			configHealthy = append(configHealthy, s)
		} else {
			discoveredHealthy = append(discoveredHealthy, s)
		}
	}

	sort.Slice(configHealthy, func(i, j int) bool {
		return configHealthy[i].Latency < configHealthy[j].Latency
	})
	sort.Slice(discoveredHealthy, func(i, j int) bool {
		return discoveredHealthy[i].Latency < discoveredHealthy[j].Latency
	})

	targets := make([]string, 0, 3)

	// 1. Un serveur de confiance (.env) en priorité s'il est sain
	if len(configHealthy) > 0 {
		targets = append(targets, configHealthy[0].URL)
	}

	// 2. 1 ou 2 relais découverts aléatoirement parmi le top 5 des plus rapides
	if len(discoveredHealthy) > 0 {
		poolSize := min(5, len(discoveredHealthy))
		pool := make([]*ServerStatus, poolSize)
		copy(pool, discoveredHealthy[:poolSize])
		shuffleServers(pool)

		for _, s := range pool {
			if len(targets) >= 3 {
				break
			}
			if !contains(targets, s.URL) {
				targets = append(targets, s.URL)
			}
		}
	}

	// 3. Compléter si nécessaire avec d'autres serveurs de config pour avoir au moins 2 cibles
	for _, s := range configHealthy {
		if len(targets) >= 2 {
			break
		}
		if !contains(targets, s.URL) {
			targets = append(targets, s.URL)
		}
	}

	if len(targets) == 0 {
		return c.seedURLs
	}
	return targets
}

// RefreshServers interroge Nostr, ping tous les serveurs et reconstruit le pool actif.
func (c *Client) RefreshServers(ctx context.Context) {
	discovered := discoverFromNostr(ctx)

	c.mu.Lock()
	now := time.Now()

	// Réhabilitation des serveurs dont le délai de quarantaine est dépassé
	for _, s := range c.servers {
		if s.Blacklisted && !s.BlacklistUntil.IsZero() && now.After(s.BlacklistUntil) {
			s.Blacklisted = false
			s.BlacklistReason = ""
			s.BlacklistUntil = time.Time{}
		}
	}

	// Ajout des nouveaux nœuds découverts
	for _, u := range discovered {
		if _, exists := c.servers[u]; !exists {
			c.servers[u] = &ServerStatus{
				URL:    u,
				Host:   publicHost(u),
				Source: "Discovered",
			}
		}
	}

	// Plafonnement strict de la liste des serveurs découverts
	c.pruneDiscoveredServers()
	c.mu.Unlock()

	// Vérification de santé sur tous les serveurs non blacklistés
	var wg sync.WaitGroup
	c.mu.RLock()
	for _, status := range c.servers {
		if status.Blacklisted {
			continue
		}
		wg.Add(1)
		go func(s *ServerStatus) {
			defer wg.Done()
			c.checkServerHealth(ctx, s)
		}(status)
	}
	c.mu.RUnlock()
	wg.Wait()

	// Réorganisation des URLs actives
	all := c.GetAllServers()
	var newActive []string
	for _, s := range all {
		if s.Online && !s.Blacklisted {
			newActive = append(newActive, s.URL)
		}
	}

	c.mu.Lock()
	if len(newActive) > 0 {
		c.activeURLs = newActive
	} else {
		c.activeURLs = c.seedURLs
	}
	c.mu.Unlock()
}

// pruneDiscoveredServers élimine les instances découvertes les moins performantes
// pour ne conserver que les 50 meilleures. Les serveurs "Config" ne sont jamais purgés.
func (c *Client) pruneDiscoveredServers() {
	var discovered []*ServerStatus
	for _, s := range c.servers {
		if s.Source == "Discovered" {
			discovered = append(discovered, s)
		}
	}

	if len(discovered) <= maxDiscoveredServers {
		return
	}

	// Tri du meilleur au pire :
	// 1. Non blacklisté d'abord
	// 2. En ligne d'abord
	// 3. Plus faible latence
	// 4. Dernier ping le plus récent
	sort.Slice(discovered, func(i, j int) bool {
		if discovered[i].Blacklisted != discovered[j].Blacklisted {
			return !discovered[i].Blacklisted
		}
		if discovered[i].Online != discovered[j].Online {
			return discovered[i].Online
		}
		if discovered[i].Latency > 0 && discovered[j].Latency > 0 && discovered[i].Latency != discovered[j].Latency {
			return discovered[i].Latency < discovered[j].Latency
		}
		return discovered[i].LastChecked.After(discovered[j].LastChecked)
	})

	// Suppression des instances excédentaires (les moins fiables)
	for _, s := range discovered[maxDiscoveredServers:] {
		delete(c.servers, s.URL)
	}
}

// StartDiscovery démarre la tâche périodique de mise à jour.
func (c *Client) StartDiscovery(ctx context.Context, interval time.Duration) {
	go func() {
		c.RefreshServers(ctx)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				c.RefreshServers(ctx)
			}
		}
	}()
}

func (c *Client) Upload(ctx context.Context, src io.ReadSeeker, size int64, hash string) error {
	urls := c.SelectUploadTargets()
	if len(urls) == 0 {
		return errors.New("no Blossom instances configured")
	}

	uploaded := 0
	var firstErr error
	for _, baseURL := range urls {
		if err := c.uploadOne(ctx, baseURL, src, size, hash); err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("%s: %w", publicHost(baseURL), err)
			}
			continue
		}
		uploaded++
	}
	if uploaded > 0 {
		return nil
	}
	if firstErr != nil {
		return fmt.Errorf("all target Blossom instances rejected the upload: %w", firstErr)
	}
	return errors.New("all target Blossom instances failed")
}

func (c *Client) uploadOne(ctx context.Context, baseURL string, src io.ReadSeeker, size int64, hash string) error {
	if err := relayURLAllowed(ctx, baseURL, c.allowPrivateRelays); err != nil {
		return fmt.Errorf("unsafe relay URL: %w", err)
	}
	if _, err := src.Seek(0, io.SeekStart); err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, strings.TrimRight(baseURL, "/")+"/upload", src)
	if err != nil {
		return err
	}
	req.ContentLength = size
	req.Header.Set("User-Agent", c.UserAgent)
	// The payload is opaque ciphertext, so it must remain a generic blob.
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("X-SHA-256", hash)

	auth, err := authorization(baseURL, hash)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", auth)

	resp, err := c.safeHTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		errMsg := responseError(resp, body)
		if isBlacklistableStatus(resp.StatusCode) {
			c.blacklistDiscoveredServer(baseURL, fmt.Sprintf("HTTP %d: %s", resp.StatusCode, errMsg))
		}
		return errors.New(errMsg)
	}
	if err := validateDescriptor(body, hash); err != nil {
		return err
	}
	return nil
}

func (c *Client) Fetch(ctx context.Context, hash string) (*http.Response, error) {
	urls := c.GetActiveURLs()
	var lastErr string

	for _, baseURL := range urls {
		if err := relayURLAllowed(ctx, baseURL, c.allowPrivateRelays); err != nil {
			lastErr = fmt.Sprintf("%s: unsafe relay URL: %v", publicHost(baseURL), err)
			continue
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(baseURL, "/")+"/"+hash, nil)
		if err != nil {
			lastErr = err.Error()
			continue
		}
		req.Header.Set("User-Agent", c.UserAgent)
		req.Header.Set("Accept", "application/octet-stream")
		resp, err := c.safeHTTP.Do(req)
		if err != nil {
			lastErr = err.Error()
			continue
		}
		if resp.StatusCode == http.StatusOK {
			return resp, nil
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			lastErr = fmt.Sprintf("%s returned HTTP %d: %s", publicHost(baseURL), resp.StatusCode, strings.TrimSpace(string(body)))
		}
	}
	if lastErr == "" {
		return nil, ErrNotFound
	}
	return nil, fmt.Errorf("all Blossom instances failed: %s", lastErr)
}

func isBlacklistableStatus(code int) bool {
	return code == http.StatusBadRequest ||
		code == http.StatusUnauthorized ||
		code == http.StatusPaymentRequired ||
		code == http.StatusForbidden ||
		code == http.StatusRequestEntityTooLarge ||
		code == http.StatusUnsupportedMediaType ||
		code == http.StatusUnprocessableEntity
}

func (c *Client) blacklistDiscoveredServer(serverURL, reason string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	s, exists := c.servers[serverURL]
	if !exists {
		return
	}
	if s.Source == "Discovered" {
		s.Blacklisted = true
		s.Online = false
		s.BlacklistReason = reason
		s.BlacklistUntil = time.Now().Add(blacklistDuration)

		filtered := make([]string, 0, len(c.activeURLs))
		for _, u := range c.activeURLs {
			if u != serverURL {
				filtered = append(filtered, u)
			}
		}
		c.activeURLs = filtered
	}
}

func shuffleServers(slice []*ServerStatus) {
	for i := len(slice) - 1; i > 0; i-- {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		if err != nil {
			continue
		}
		j := int(n.Int64())
		slice[i], slice[j] = slice[j], slice[i]
	}
}

func contains(slice []string, val string) bool {
	for _, item := range slice {
		if item == val {
			return true
		}
	}
	return false
}

func validateDescriptor(body []byte, expectedHash string) error {
	if len(body) == 0 {
		return nil
	}

	var descriptor Descriptor
	if err := json.Unmarshal(body, &descriptor); err != nil || descriptor.SHA256 == "" {
		return nil
	}
	if !strings.EqualFold(descriptor.SHA256, expectedHash) {
		return errors.New("server returned a different blob hash")
	}
	return nil
}

func responseError(resp *http.Response, body []byte) string {
	if reason := strings.TrimSpace(resp.Header.Get("X-Reason")); reason != "" {
		return reason
	}
	message := strings.TrimSpace(string(body))
	if message != "" && len(message) < 512 {
		return message
	}
	return fmt.Sprintf("Blossom returned HTTP %d", resp.StatusCode)
}

func publicHost(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	return u.Hostname()
}

// UploadDistributedShards distribue les N shards sur au moins 2 ou 3 relais distincts.
func (c *Client) UploadDistributedShards(ctx context.Context, shards [][]byte, shardHashes []string) ([]string, error) {
	targets := c.SelectUploadTargets()
	if len(targets) == 0 {
		targets = c.GetActiveURLs()
	}
	if len(targets) == 0 {
		return nil, errors.New("no Blossom instances available for distributed upload")
	}

	// Si moins de 2 cibles, tenter d'ajouter d'autres serveurs actifs pour garantir au moins 2 ou 3 relais distincts
	if len(targets) < 2 {
		for _, u := range c.GetActiveURLs() {
			if !contains(targets, u) {
				targets = append(targets, u)
			}
			if len(targets) >= 3 {
				break
			}
		}
	}

	usedRelaysMap := make(map[string]bool)
	var mu sync.Mutex

	for i, shardData := range shards {
		hash := shardHashes[i]
		size := int64(len(shardData))
		uploaded := false

		// Cible prioritaire répartie en round-robin
		primaryTarget := targets[i%len(targets)]
		orderedTargets := make([]string, 0, len(targets))
		orderedTargets = append(orderedTargets, primaryTarget)
		for _, t := range targets {
			if t != primaryTarget {
				orderedTargets = append(orderedTargets, t)
			}
		}

		var lastErr error
		for _, baseURL := range orderedTargets {
			reader := bytes.NewReader(shardData)
			if err := c.uploadOne(ctx, baseURL, reader, size, hash); err != nil {
				lastErr = err
				continue
			}
			mu.Lock()
			usedRelaysMap[baseURL] = true
			mu.Unlock()
			uploaded = true
			break
		}

		if !uploaded {
			return nil, fmt.Errorf("failed to upload shard %d: %w", i, lastErr)
		}
	}

	usedRelays := make([]string, 0, len(usedRelaysMap))
	for r := range usedRelaysMap {
		usedRelays = append(usedRelays, r)
	}
	sort.Strings(usedRelays)
	return usedRelays, nil
}

// publicRelayURL rejects relay URLs that could reach loopback, private, link-local,
// multicast, unspecified, or otherwise non-global IP addresses. DNS answers are
// checked before use; redirects are separately disabled for shard requests.
func publicRelayURL(ctx context.Context, raw string) error {
	return relayURLAllowed(ctx, raw, false)
}

func relayURLAllowed(ctx context.Context, raw string, allowPrivate bool) error {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Hostname() == "" || u.User != nil {
		return errors.New("invalid relay URL")
	}
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	if !allowPrivate {
		for _, suffix := range []string{".localhost", ".local", ".internal", ".lan", ".home.arpa"} {
			if host == strings.TrimPrefix(suffix, ".") || strings.HasSuffix(host, suffix) {
				return errors.New("non-public relay hostname")
			}
		}
	}
	var ips []net.IP
	if ip := net.ParseIP(host); ip != nil {
		ips = []net.IP{ip}
	} else {
		answers, lookupErr := net.DefaultResolver.LookupIPAddr(ctx, host)
		if lookupErr != nil || len(answers) == 0 {
			return errors.New("relay hostname did not resolve")
		}
		for _, answer := range answers {
			ips = append(ips, answer.IP)
		}
	}
	if !allowPrivate {
		for _, ip := range ips {
			if !isPublicRelayIP(ip) {
				return errors.New("relay resolves to a non-public IP address")
			}
		}
	}
	return nil
}

// FetchInnovativeShards télécharge en parallèle des shards parmi les relais connus,
// et annule les requêtes restantes dès que K shards valides et linéairement indépendants sont reçus.
func (c *Client) FetchInnovativeShards(ctx context.Context, shardHashes []string, relays []string, k int, maxShardBytes int64) ([]badrlnc.Shard, error) {
	// Bound attacker-controlled manifest fields before starting goroutines or requests.
	if k <= 0 || k > 64 || len(shardHashes) < k || len(shardHashes) > 96 ||
		len(relays) > maxDiscoveredServers || maxShardBytes <= 40 || maxShardBytes > (2<<20) {
		return nil, errors.New("invalid RLNC manifest limits")
	}
	for _, hash := range shardHashes {
		decoded, err := hex.DecodeString(hash)
		if err != nil || len(decoded) != sha256.Size {
			return nil, errors.New("invalid shard hash")
		}
	}

	fetchCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	candidateRelays := make([]string, 0, len(relays))
	for _, r := range relays {
		if r != "" && !contains(candidateRelays, r) && relayURLAllowed(fetchCtx, r, c.allowPrivateRelays) == nil {
			candidateRelays = append(candidateRelays, r)
		}
	}
	for _, r := range c.GetActiveURLs() {
		if !contains(candidateRelays, r) && relayURLAllowed(fetchCtx, r, c.allowPrivateRelays) == nil {
			candidateRelays = append(candidateRelays, r)
		}
	}
	if len(candidateRelays) == 0 {
		return nil, errors.New("no safe public relays available to fetch shards")
	}

	// Reuse the shared client whose dialer blocks private destinations and whose
	// redirect policy prevents a public relay from redirecting to a private target.
	safeClient := c.safeHTTP

	type shardResult struct {
		shard badrlnc.Shard
		hash  string
		err   error
	}

	resChan := make(chan shardResult, len(shardHashes))
	var wg sync.WaitGroup
	fetchSlots := make(chan struct{}, 8)

	for i, h := range shardHashes {
		wg.Add(1)
		go func(idx int, targetHash string) {
			defer wg.Done()

			ordered := make([]string, 0, len(candidateRelays))
			start := idx % len(candidateRelays)
			for j := 0; j < len(candidateRelays); j++ {
				ordered = append(ordered, candidateRelays[(start+j)%len(candidateRelays)])
			}

			var lastErr error
			for _, baseURL := range ordered {
				select {
				case <-fetchCtx.Done():
					return
				default:
				}

				select {
				case fetchSlots <- struct{}{}:
				case <-fetchCtx.Done():
					return
				}
				req, err := http.NewRequestWithContext(fetchCtx, http.MethodGet, strings.TrimRight(baseURL, "/")+"/"+targetHash, nil)
				if err != nil {
					<-fetchSlots
					lastErr = err
					continue
				}
				req.Header.Set("User-Agent", c.UserAgent)
				req.Header.Set("Accept", "application/octet-stream")

				resp, err := safeClient.Do(req)
				if err != nil {
					<-fetchSlots
					lastErr = err
					continue
				}

				if resp.StatusCode != http.StatusOK {
					resp.Body.Close()
					<-fetchSlots
					lastErr = fmt.Errorf("HTTP %d from %s", resp.StatusCode, baseURL)
					continue
				}

				body, err := io.ReadAll(io.LimitReader(resp.Body, maxShardBytes+1))
				resp.Body.Close()
				<-fetchSlots
				if err != nil {
					lastErr = err
					continue
				}
				if int64(len(body)) > maxShardBytes {
					lastErr = errors.New("shard exceeds declared size limit")
					continue
				}

				hasher := sha256.New()
				hasher.Write(body)
				calcHash := hex.EncodeToString(hasher.Sum(nil))
				if !strings.EqualFold(calcHash, targetHash) {
					lastErr = errors.New("shard hash mismatch")
					continue
				}

				shard, err := rlncbox.DeserializeShard(body)
				if err != nil {
					lastErr = err
					continue
				}

				select {
				case resChan <- shardResult{shard: shard, hash: targetHash}:
					return
				case <-fetchCtx.Done():
					return
				}
			}

			if lastErr == nil {
				lastErr = errors.New("shard not found on any relay")
			}
			select {
			case resChan <- shardResult{err: lastErr, hash: targetHash}:
			case <-fetchCtx.Done():
			}
		}(i, h)
	}

	go func() {
		wg.Wait()
		close(resChan)
	}()

	acceptedShards := make([]badrlnc.Shard, 0, k)
	acceptedMasks := make([]badrlnc.Bitset256, 0, k)

	for res := range resChan {
		if res.err != nil {
			continue
		}
		if rlncbox.IsInnovative(acceptedMasks, res.shard.Mask) {
			acceptedShards = append(acceptedShards, res.shard)
			acceptedMasks = append(acceptedMasks, res.shard.Mask)

			if len(acceptedShards) >= k {
				cancel()
				return acceptedShards, nil
			}
		}
	}

	if len(acceptedShards) < k {
		return nil, fmt.Errorf("insufficient innovative shards: got %d, need %d", len(acceptedShards), k)
	}
	return acceptedShards, nil
}
