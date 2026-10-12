package main

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"anonblossom/internal/web"
)

func TestResolvePageVersionRequiresReleaseTag(t *testing.T) {
	for _, valid := range []string{"v0.2.34", "v10.0.1"} {
		if got, err := resolveVersion(valid); err != nil || got != valid {
			t.Fatalf("valid version rejected: got %q, err %v", got, err)
		}
	}
	for _, invalid := range []string{"dev", "main", "deadbeef", "v0.2.34<svg>", "v0.2", "v1.2.3-rc1"} {
		if _, err := resolveVersion(invalid); err == nil {
			t.Fatalf("invalid static version was accepted: %q", invalid)
		}
	}
}

func TestParseRelaysRejectsUnsafeOrDuplicateHosts(t *testing.T) {
	good := "https://one.example,https://two.example,https://three.example,https://four.example"
	got, err := parseRelays(good)
	if err != nil || len(got) != 4 {
		t.Fatalf("valid independent HTTPS relay list rejected: %v", err)
	}
	defaults, err := parseRelays(defaultRelays)
	if err != nil || len(defaults) < 5 {
		t.Fatalf("expanded default Blossom storage list invalid: %v", err)
	}
	if len(publicBlossomDirectory) < 5 {
		t.Fatal("public directory must contain multiple additional Blossom services")
	}
	var dozen []string
	for i := 0; i < maxStaticStorageRelays; i++ {
		dozen = append(dozen, fmt.Sprintf("https://relay-%d.example", i))
	}
	if got, err := parseRelays(strings.Join(dozen, ",")); err != nil || len(got) != maxStaticStorageRelays {
		t.Fatalf("maximum supported static relay list invalid: %v", err)
	}
	for _, raw := range []string{
		"http://one.example,https://two.example",
		"https://one.example/private,https://two.example",
		"https://one.example?key=abc,https://two.example",
		"https://user:password@one.example,https://two.example",
		"https://one.example,https://ONE.example",
		"https://one.example",
		strings.Join(append(dozen, "https://extra.example"), ","),
		"https://one.example,https://two.example#secret",
	} {
		if _, err := parseRelays(raw); err == nil {
			t.Fatalf("unsafe or unsupported relay list was accepted: %q", raw)
		}
	}
}

func TestStaticPagesHaveBrowserOnlyPaths(t *testing.T) {
	oldVersion := web.Version
	web.Version = "v9.8.7"
	t.Cleanup(func() { web.Version = oldVersion })
	pages, err := web.NewPages()
	if err != nil {
		t.Fatal(err)
	}
	home, view, err := renderPages(pages)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		`href="./static/pico.min.css"`,
		`src="./static/config.js"`,
		`src="./static/wasm_exec.js"`,
		`src="./static/direct-blossom.js"`,
		`src="./static/boot.js"`,
		`href="./relays.html"`,
		`action="./"`,
		`id="submitBtn" class="btn-action" disabled`,
		`64.0 MiB`,
		`<span class="app-version">v9.8.7</span>`,
		"Direct Blossom transfers require CORS",
	} {
		if !strings.Contains(home, required) {
			t.Fatalf("static upload page missing %q", required)
		}
	}
	if strings.Contains(home, `id="proxyOnly"`) || strings.Contains(home, `action="/upload"`) ||
		strings.Contains(home, `href="/relays"`) {
		t.Fatal("static home must not offer server-dependent features")
	}
	for _, required := range []string{
		`src="./static/config.js"`,
		`src="./static/view.js" data-cipher=""`,
		`id="zkLoader"`,
		`id="zkContent"`,
		`href="./"`,
		`This file was shared in Zero-Knowledge mode`,
	} {
		if !strings.Contains(view, required) {
			t.Fatalf("static viewer missing %q", required)
		}
	}
	if strings.Contains(view, `id="proxyOnly"`) || strings.Contains(view, `src="/static/`) {
		t.Fatal("static viewer must not depend on proxy or domain-root paths")
	}
	for _, page := range []string{home, view} {
		if !strings.Contains(page, `href="https://ihatemyfcklife.github.io/insomniaxxx/" class="logo-link"`) {
			t.Fatal("static page logo must point to the GitHub Pages project homepage")
		}
		if strings.Contains(page, `href="/" class="logo-link"`) ||
			strings.Contains(page, `href="./" class="logo-link"`) {
			t.Fatal("static page logo must not point to a relative or domain-root path")
		}
		if !strings.Contains(page, `<span class="app-version">v9.8.7</span>`) ||
			strings.Contains(page, `<span class="app-version">dev</span>`) {
			t.Fatal("static exporter must render the actual version, never dev")
		}
		if !strings.Contains(page, `http-equiv="Content-Security-Policy"`) {
			t.Fatal("static page has no CSP")
		}
	}
}

func TestStaticRelaysDoNotClaimLiveStatus(t *testing.T) {
	pages, err := web.NewPages()
	if err != nil {
		t.Fatal(err)
	}
	page, err := renderRelayPage(pages, []string{"https://one.example", "https://two.example"})
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"Blossom Relays",
		"Configured storage relays and public Blossom directory.",
		"Directory entries are reference-only",
		"Availability and browser CORS are not checked",
		`class="relays-card"`,
		`class="relays-table-wrapper"`,
		`class="relays-table" role="grid"`,
		`<th scope="col">Server</th>`,
		`<th scope="col">Status</th>`,
		`<th scope="col">Latency</th>`,
		`<th scope="col">Source</th>`,
		`<th scope="col">Last Check</th>`,
		`class="relay-row"`,
		`class="relay-badge unchecked">Not checked</span>`,
		`class="relay-source"`,
		`class="relay-check"`,
		`class="relay-latency"`,
		`>Configured</small>`,
		`href="./static/pico.min.css"`,
		`href="./"`,
		`href="https://ihatemyfcklife.github.io/insomniaxxx/" class="logo-link"`,
		`href="https://one.example"`,
		`href="https://two.example"`,
		"Only entries marked Configured are used for direct transfers.",
	} {
		if !strings.Contains(page, required) {
			t.Fatalf("static relay page missing %q", required)
		}
	}
	if got, want := strings.Count(page, `class="relay-row"`), 2+len(publicBlossomDirectory); got != want {
		t.Fatalf("static relay table must include configured plus public directory servers: got %d, want %d", got, want)
	}
	if strings.Count(page, `>Configured</small>`) != 2 ||
		strings.Count(page, `>Directory</small>`) != len(publicBlossomDirectory) {
		t.Fatal("configured active relays must be distinguished from reference-only directory servers")
	}
	// An origin configured for storage must not be duplicated in the directory.
	withDuplicate, err := renderRelayPage(pages, []string{
		"https://one.example", publicBlossomDirectory[0],
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(withDuplicate, `class="relay-row"`) != 2+len(publicBlossomDirectory)-1 {
		t.Fatal("directory must not duplicate configured storage origins")
	}
	for _, forbidden := range []string{
		`href="/" class="logo-link"`,
		`href="/relays"`,
		`<a role="button" class="secondary" href="/relays">Refresh</a>`,
		"Auto-discovered via Nostr",
		`class="relay-badge online"`,
		`class="relay-badge offline"`,
		`class="relay-badge blacklisted"`,
	} {
		if strings.Contains(page, forbidden) {
			t.Fatalf("static page must not claim live relay monitoring: %q", forbidden)
		}
	}
	if !strings.Contains(web.PicoThemeCSS, `@media (max-width: 680px)`) ||
		!strings.Contains(web.PicoThemeCSS, `tr.relay-row {`) ||
		!strings.Contains(web.PicoThemeCSS, `.relay-badge.unchecked {`) {
		t.Fatal("shared responsive relay-card styling must support unchecked static relays")
	}

	// The standard clearnet and Tor relay pages must retain live status labels
	// and the refresh action.
	live := httptest.NewRecorder()
	if err := pages.Relays(live, web.PageData{
		Title: "insomniaXXX",
		Relays: []web.RelayView{{URL: "https://one.example", Online: true, Source: "Nostr"}},
	}); err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"Active storage instances discovered and monitored across Nostr.",
		`class="relay-badge online">Online</span>`,
		`href="/relays">Refresh</a>`,
	} {
		if !strings.Contains(live.Body.String(), required) {
			t.Fatalf("server-hosted relay view changed unexpectedly: %q", required)
		}
	}
}
