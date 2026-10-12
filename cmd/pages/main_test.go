package main

import (
	"strings"
	"testing"

	"anonblossom/internal/web"
)

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
		`href="https://one.example"`,
		`href="https://two.example"`,
	} {
		if !strings.Contains(page, required) {
			t.Fatalf("static relay page missing %q", required)
		}
	}
	if strings.Contains(page, "Auto-discovered via Nostr") || strings.Contains(page, "Online</span>") {
		t.Fatal("static site must not advertise server-monitored relay status")
	}
}
