package main

import (
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
	good := "https://one.example,https://two.example,https://three.example"
	got, err := parseRelays(good)
	if err != nil || len(got) != 3 {
		t.Fatalf("valid independent HTTPS relay list rejected: %v", err)
	}
	for _, raw := range []string{
		"http://one.example,https://two.example",
		"https://one.example/private,https://two.example",
		"https://one.example?key=abc,https://two.example",
		"https://user:password@one.example,https://two.example",
		"https://one.example,https://ONE.example",
		"https://one.example",
		"https://one.example,https://two.example,https://three.example,https://four.example",
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
	page := renderRelayPage([]string{"https://one.example", "https://two.example"})
	for _, required := range []string{
		"Configured Blossom Relays",
		"availability and browser CORS permissions are not guaranteed",
		`href="./"`,
		`href="https://ihatemyfcklife.github.io/insomniaxxx/" class="logo-link"`,
		`href="https://one.example"`,
		`href="https://two.example"`,
	} {
		if !strings.Contains(page, required) {
			t.Fatalf("static relay page missing %q", required)
		}
	}
	if strings.Contains(page, `href="/" class="logo-link"`) {
		t.Fatal("relay page logo must not navigate to the domain root")
	}
	if strings.Contains(page, "Auto-discovered via Nostr") || strings.Contains(page, "Online</span>") {
		t.Fatal("static site must not advertise server-monitored relay status")
	}
}
