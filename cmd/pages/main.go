// Command pages exports the existing Zero-Knowledge UI as a fully static site.
// No Go server, application API, CAPTCHA or encrypted proxy is needed at runtime.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"anonblossom/internal/web"
)

const defaultRelays = "https://cdn.hzrd149.com,https://nostr.download,https://blossom.poster.place"
const maxUploadBytes = 64 << 20

var staticAssets = []string{
	"app.wasm", "wasm_exec.js", "pico.min.css", "favicon.ico",
	"favicon.png", "logo.png", "xxx.png", "loading.gif",
	"preview-unavailable.png", "boot.js", "direct-blossom.js",
	"stream-file.js", "view.js",
}

func main() {
	output := flag.String("out", "dist/pages", "output directory for static GitHub Pages site")
	relayArg := flag.String("relays", defaultRelays, "2 or 3 independent HTTPS Blossom origins (comma-separated)")
	versionArg := flag.String("version", "", "version tag displayed in the footer (default: latest Git tag)")
	flag.Parse()

	version, err := resolveVersion(*versionArg)
	if err == nil {
		web.Version = version
		var relays []string
		relays, err = parseRelays(*relayArg)
		if err == nil {
			err = build(*output, relays)
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "pages:", err)
		os.Exit(1)
	}
	fmt.Printf("Static Zero-Knowledge site generated in %s\n", *output)
}

// resolveVersion uses the latest reachable release tag. Fail rather than
// publish a static Pages site with a misleading "dev" footer.
func resolveVersion(explicit string) (string, error) {
	version := strings.TrimSpace(explicit)
	if version == "" {
		output, err := exec.Command("git", "describe", "--tags", "--abbrev=0", "--match", "v[0-9]*").Output()
		if err != nil {
			return "", errors.New("cannot determine release version from Git tags; pass -version vX.Y.Z")
		}
		version = strings.TrimSpace(string(output))
	}
	if !regexp.MustCompile("^v[0-9]+[.][0-9]+[.][0-9]+$").MatchString(version) {
		return "", fmt.Errorf("invalid release version %q (expected vX.Y.Z)", version)
	}
	return version, nil
}

func parseRelays(raw string) ([]string, error) {
	parts := strings.Split(raw, ",")
	if len(parts) < 2 || len(parts) > 3 {
		return nil, errors.New("provide 2 or 3 independent HTTPS Blossom relay origins")
	}
	relays := make([]string, 0, len(parts))
	seen := map[string]bool{}
	for _, part := range parts {
		origin := strings.TrimSpace(part)
		u, err := url.Parse(origin)
		if err != nil || u.Scheme != "https" || u.Hostname() == "" ||
			u.User != nil || u.RawQuery != "" || u.Fragment != "" ||
			(u.Path != "" && u.Path != "/") || u.Opaque != "" {
			return nil, fmt.Errorf("invalid HTTPS relay origin: %q", origin)
		}
		host := strings.ToLower(u.Hostname())
		if seen[host] {
			return nil, fmt.Errorf("duplicate Blossom host: %q", host)
		}
		seen[host] = true
		relays = append(relays, u.Scheme+"://"+u.Host)
	}
	return relays, nil
}

func renderPages(pages *web.Pages) (string, string, error) {
	home := httptest.NewRecorder()
	if err := pages.Home(home, web.PageData{
		Title: "insomniaXXX", MaxUpload: "64.0 MiB", MaxUploadBytes: maxUploadBytes,
	}); err != nil {
		return "", "", err
	}
	view := httptest.NewRecorder()
	if err := pages.View(view, web.PageData{
		Title: "insomniaXXX", FileName: "Shared file", ZK: true,
	}); err != nil {
		return "", "", err
	}

	homeHTML, err := removeHTMLBlock(home.Body.String(), `<div class="upload-note" id="proxyOptions" hidden>`)
	if err != nil {
		return "", "", err
	}
	// Explicitly explain the reduced transport privacy of the browser-only edition.
	homeHTML = strings.Replace(homeHTML, "</form>",
		`</form>
    <div class="upload-note"><small class="muted">Static Zero-Knowledge edition: encryption stays in your browser. Direct Blossom transfers require CORS and reveal your network address to the relays. No application proxy is available.</small></div>`, 1)
	homeHTML = strings.Replace(homeHTML,
		"Use the onion service for server processing without JavaScript.",
		"This static edition needs JavaScript and WebAssembly for uploads.", 1)
	homeHTML = strings.Replace(homeHTML, `action="/upload"`, `action="./"`, 1)
	homeHTML = strings.Replace(homeHTML, `href="/relays"`, `href="./relays.html"`, 1)

	viewHTML, err := removeHTMLBlock(view.Body.String(), `<div id="zkProxyOptions" class="view-note">`)
	if err != nil {
		return "", "", err
	}
	// The static viewer reads its ciphertext hash from ?hash=, not from Go.
	return makeStatic(homeHTML), makeStatic(viewHTML), nil
}

func removeHTMLBlock(page, marker string) (string, error) {
	start := strings.Index(page, marker)
	if start < 0 {
		return "", fmt.Errorf("missing HTML block: %s", marker)
	}
	end := strings.Index(page[start:], "</div>")
	if end < 0 {
		return "", fmt.Errorf("unclosed HTML block: %s", marker)
	}
	return page[:start] + page[start+end+len("</div>"):], nil
}

func makeStatic(page string) string {
	// Project Pages live at /<repository>/, never at the domain root.
	page = strings.ReplaceAll(page, "/static/", "./static/")
	page = strings.ReplaceAll(page, `href="/"`, `href="./"`)
	page = strings.ReplaceAll(page, `href="/favicon.ico"`, `href="./static/favicon.ico"`)
	page = strings.Replace(page, `<script src="./static/direct-blossom.js"></script>`,
		`<script src="./static/config.js"></script>
  <script src="./static/direct-blossom.js"></script>`, 1)
	page = strings.Replace(page, "<head>", "<head>\n  <meta http-equiv=\"Content-Security-Policy\" content=\""+web.PicoCSP+"\">", 1)
	return page
}

func renderRelayPage(relays []string) string {
	var entries strings.Builder
	for _, relay := range relays {
		target := html.EscapeString(relay)
		fmt.Fprintf(&entries, `<li><a href="%s" target="_blank" rel="noopener noreferrer">%s</a></li>`, target, target)
	}
	return fmt.Sprintf(`<!doctype html>
<html lang="en" data-theme="dark">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <meta name="referrer" content="no-referrer">
  <meta http-equiv="Content-Security-Policy" content="%s">
  <title>Blossom Relays · insomniaXXX</title>
  <link rel="icon" type="image/png" href="./static/favicon.png">
  <link rel="stylesheet" href="./static/pico.min.css">
  <style>%s</style>
</head>
<body><main class="container">
%s
<header><h1>Configured Blossom Relays</h1><p class="muted">These HTTPS storage destinations are configured at build time; their availability and browser CORS permissions are not guaranteed.</p></header>
<article><ul>%s</ul></article>
<p><a href="./" class="view-relays-link">Back to upload</a></p>
%s
</main></body></html>`, web.PicoCSP, web.PicoThemeCSS,
		strings.ReplaceAll(web.LogoHTML(), "/static/", "./static/"), entries.String(), web.FooterHTML())
}

func build(out string, relays []string) error {
	pages, err := web.NewPages()
	if err != nil {
		return err
	}
	home, view, err := renderPages(pages)
	if err != nil {
		return err
	}
	config, err := json.Marshal(struct {
		Enabled bool     `json:"enabled"`
		Relays  []string `json:"relays"`
	}{Enabled: true, Relays: relays})
	if err != nil {
		return err
	}
	static := filepath.Join(out, "static")
	if err := os.MkdirAll(static, 0755); err != nil {
		return err
	}
	outputs := map[string]string{
		"index.html": home,
		"view.html": view,
		"relays.html": renderRelayPage(relays),
		".nojekyll": "",
	}
	for name, content := range outputs {
		if err := os.WriteFile(filepath.Join(out, name), []byte(content), 0644); err != nil {
			return err
		}
	}
	if err := os.WriteFile(filepath.Join(static, "config.js"),
		append(append([]byte("window.InsomniaStatic = Object.freeze("), config...), []byte(");\n")...), 0644); err != nil {
		return err
	}
	for _, name := range staticAssets {
		source := filepath.Join("internal", "app", "static", name)
		bytes, err := os.ReadFile(source)
		if err != nil {
			return fmt.Errorf("missing required static asset %s: %w (build WASM first)", name, err)
		}
		if len(bytes) == 0 {
			return fmt.Errorf("empty static asset: %s", name)
		}
		if err := os.WriteFile(filepath.Join(static, name), bytes, 0644); err != nil {
			return err
		}
	}
	return nil
}
