package web

import (
	"html/template"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestDoneRendersInlineQRAndTheme(t *testing.T) {
	pages, err := NewPages()
	if err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	err = pages.Done(w, PageData{
		Title:    "drop",
		ShareURL: "https://files.example/d/hash/key",
		FileName: "example.txt",
		FileSize: "12 B",
		QRCode:   template.HTML(`<img src="/qr?data=example" alt="QR code">`),
	})
	if err != nil {
		t.Fatal(err)
	}

	body := w.Body.String()
	for _, needle := range []string{
		`/static/pico.min.css`,
		`/static/favicon.png`,
		`/favicon.ico`,
		`--pico-primary`,
		`<img src="/qr?data=example" alt="QR code">`,
		`role="status"`,
		`logo-box`,
	} {
		if !strings.Contains(body, needle) {
			t.Fatalf("rendered page missing %q", needle)
		}
	}
}

func TestUnavailablePreviewShowsCrossInBothDownloadModes(t *testing.T) {
	pages, err := NewPages()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		data PageData
	}{
		{"zero-knowledge", PageData{Title: "insomniaXXX", ZK: true}},
		{"server-decrypted", PageData{Title: "insomniaXXX"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			if err := pages.View(rec, tc.data); err != nil {
				t.Fatal(err)
			}
			body := rec.Body.String()
			message := `No preview available for this file format.</p>`
			image := `<img class="no-preview-cross" src="/static/preview-unavailable.png" alt="" width="128" height="128" loading="lazy" draggable="false">`
			messageAt := strings.Index(body, message)
			imageAt := strings.Index(body, image)
			if messageAt < 0 || imageAt <= messageAt {
				t.Fatal("large no-preview PNG must follow the fallback message")
			}
		})
	}
}

func TestPreviewMascotAnchoredAbovePreview(t *testing.T) {
	pages, err := NewPages()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		data PageData
	}{
		{"zero-knowledge", PageData{Title: "insomniaXXX", ZK: true}},
		{"server-fallback", PageData{Title: "insomniaXXX"}},
		{"server-image", PageData{Title: "insomniaXXX", IsImage: true}},
		{"server-video", PageData{Title: "insomniaXXX", IsVideo: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			if err := pages.View(rec, tc.data); err != nil {
				t.Fatal(err)
			}
			body := rec.Body.String()
			frame := strings.Index(body, `id="previewFrame" class="preview-frame"`)
			mascot := strings.Index(body, `<img class="preview-mascot" src="/static/xxx.png" alt="" width="1024" height="1072" draggable="false" aria-hidden="true">`)
			preview := strings.Index(body, `id="previewBox" class="preview-box"`)
			if frame < 0 || mascot <= frame || preview <= mascot {
				t.Fatal("non-draggable mascot must be inside the preview frame, before the media")
			}
			if strings.Count(body, `class="preview-mascot"`) != 1 {
				t.Fatal("expected exactly one decorative mascot per viewer")
			}
		})
	}
	if !strings.Contains(PicoThemeCSS, ".preview-mascot {") ||
		!strings.Contains(PicoThemeCSS, "bottom: 100%") ||
		!strings.Contains(PicoThemeCSS, ".no-preview-cross {") {
		t.Fatal("preview mascot or cross styles missing")
	}

	mascotCSS := strings.SplitN(PicoThemeCSS, ".preview-mascot {", 2)
	if len(mascotCSS) != 2 || !strings.Contains(strings.SplitN(mascotCSS[1], "}", 2)[0], "right: 3rem;") {
		t.Fatal("desktop preview mascot must match the home-page right: 3rem offset")
	}
}

func TestZKLoadingGIFCannotBeSelectedOrDragged(t *testing.T) {
	pages, err := NewPages()
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	if err := pages.View(rec, PageData{Title: "insomniaXXX", ZK: true}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rec.Body.String(), `id="zkSpinner" width="192" height="192" draggable="false"`) {
		t.Fatal("ZK loading GIF must have draggable=false")
	}
	rules := strings.SplitN(PicoThemeCSS, ".zk-loading-gif {", 2)
	if len(rules) != 2 {
		t.Fatal("ZK GIF CSS selector missing")
	}
	styles := strings.SplitN(rules[1], "}", 2)[0]
	for _, property := range []string{"pointer-events: none;", "user-select: none;", "-webkit-user-select: none;", "-webkit-user-drag: none;"} {
		if !strings.Contains(styles, property) {
			t.Fatalf("ZK GIF must prevent user selection/dragging: %s", property)
		}
	}
}

func TestAllPagesRenderLogo(t *testing.T) {
	pages, err := NewPages()
	if err != nil {
		t.Fatal(err)
	}

	testCases := []struct {
		name   string
		render func(w *httptest.ResponseRecorder) error
	}{
		{"home", func(w *httptest.ResponseRecorder) error {
			return pages.Home(w, PageData{Title: "insomniaXXX"})
		}},
		{"done", func(w *httptest.ResponseRecorder) error {
			return pages.Done(w, PageData{Title: "insomniaXXX", ShareURL: "/d/x#k"})
		}},
		{"view", func(w *httptest.ResponseRecorder) error {
			return pages.View(w, PageData{Title: "insomniaXXX", ZK: true})
		}},
		{"relays", func(w *httptest.ResponseRecorder) error {
			return pages.Relays(w, PageData{Title: "insomniaXXX"})
		}},
		{"error", func(w *httptest.ResponseRecorder) error {
			return pages.Error(w, PageData{Title: "insomniaXXX", StatusCode: 404})
		}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			if err := tc.render(rec); err != nil {
				t.Fatalf("%s failed: %v", tc.name, err)
			}
			if !strings.Contains(rec.Body.String(), `class="logo-box"`) {
				t.Fatalf("%s does not contain logo-box", tc.name)
			}
			if !strings.Contains(rec.Body.String(), `class="site-logo"`) {
				t.Fatalf("%s does not contain site-logo", tc.name)
			}
			if !strings.Contains(rec.Body.String(), `File sharing. No accounts. Decentralized relays.`) {
				t.Fatalf("%s does not contain site-tagline", tc.name)
			}
		})
	}
}

func TestLogoHTML(t *testing.T) {
	html := LogoHTML()
	if html == "" {
		t.Fatal("LogoHTML returned empty string")
	}
	for _, needle := range []string{
		`class="logo-box"`,
		`class="site-logo"`,
		`/static/logo.png`,
		`alt="InsomniaXXX"`,
		`draggable="false"`,
		`<header>`,
		`class="muted"`,
		`File sharing. No accounts. Decentralized relays.`,
	} {
		if !strings.Contains(html, needle) {
			t.Fatalf("LogoHTML missing %q", needle)
		}
	}
}

func TestFooterHTML(t *testing.T) {
	html := FooterHTML()
	if html == "" {
		t.Fatal("FooterHTML returned empty string")
	}
	for _, needle := range []string{`class="site-footer"`, `class="footer-author"`, `Made by`, `ihatemyfcklife`, `app-version`} {
		if !strings.Contains(html, needle) {
			t.Fatalf("FooterHTML missing %q", needle)
		}
	}
}

func TestFooterVersionAutoUpdate(t *testing.T) {
	oldVersion, hadVersion := Version, Version != ""
	oldEnv, hadEnv := os.LookupEnv("APP_VERSION")
	defer func() {
		Version = ""
		if !hadVersion {
			Version = ""
		} else {
			Version = oldVersion
		}
		if hadEnv {
			_ = os.Setenv("APP_VERSION", oldEnv)
		} else {
			_ = os.Unsetenv("APP_VERSION")
		}
	}()

	Version = "v9.9.9-test"
	_ = os.Unsetenv("APP_VERSION")
	if got := AppVersion(); got != "v9.9.9-test" {
		t.Fatalf("AppVersion with ldflags = %q, want %q", got, "v9.9.9-test")
	}
	if html := FooterHTML(); !strings.Contains(html, "v9.9.9-test") {
		t.Fatalf("FooterHTML does not contain updated version, got: %s", html)
	}

	// Changement à chaud sans redémarrage : le footer doit suivre.
	Version = "v9.9.10-test"
	if html := FooterHTML(); !strings.Contains(html, "v9.9.10-test") {
		t.Fatalf("FooterHTML stale after version change, got: %s", html)
	}

	// Fallback env quand pas de ldflags.
	Version = ""
	_ = os.Setenv("APP_VERSION", "v1.2.3-env")
	if html := FooterHTML(); !strings.Contains(html, "v1.2.3-env") {
		t.Fatalf("FooterHTML does not honor APP_VERSION, got: %s", html)
	}
}

func TestDumpHomePage(t *testing.T) {
	pages, err := NewPages()
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	if err := pages.Home(rec, PageData{Title: "insomniaXXX", MaxUpload: "50 MB"}); err != nil {
		t.Fatal(err)
	}
	body := rec.Body.String()
	for _, needle := range []string{
		`scrollbar-width: none`,
		`::-webkit-scrollbar`,
		`class="view-relays-link"`,
		`href="/relays"`,
	} {
		if !strings.Contains(body, needle) {
			t.Fatalf("home page missing %q", needle)
		}
	}
	for _, removed := range []string{
		`class="architecture-figure"`,
		`class="architecture-card"`,
		`id="architectureImg"`,
		`id="modeToggle"`,
		`id="architectureDetailsNoJS"`,
		`id="architectureDetailsWasm"`,
		`How <span class="brand-text">InsomniaXXX</span> Works`,
	} {
		if strings.Contains(body, removed) {
			t.Fatalf("home page still renders removed architecture element %q", removed)
		}
	}
	_ = os.WriteFile("/tmp/verify_home.html", rec.Body.Bytes(), 0644)
}

func TestDumpViewPage(t *testing.T) {
	pages, err := NewPages()
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	if err := pages.View(rec, PageData{Title: "insomniaXXX", ZK: true, CipherURL: "/raw/xyz"}); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile("/tmp/verify_view.html", rec.Body.Bytes(), 0644)
}

func TestDumpRelaysPage(t *testing.T) {
	pages, err := NewPages()
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	relays := []RelayView{
		{
			URL:         "https://blossom.primal.net",
			Host:        "blossom.primal.net",
			Online:      true,
			Blacklisted: false,
			Latency:     "42 ms",
			Source:      "seed",
			LastCheck:   "1m ago",
		},
		{
			URL:         "https://nostr.download",
			Host:        "nostr.download",
			Online:      true,
			Blacklisted: false,
			Latency:     "180 ms",
			Source:      "kind 10063",
			LastCheck:   "2m ago",
		},
		{
			URL:         "https://cdn.satellite.earth/blossom",
			Host:        "cdn.satellite.earth",
			Online:      false,
			Blacklisted: false,
			Latency:     "—",
			Source:      "kind 10063",
			LastCheck:   "5m ago",
		},
		{
			URL:         "https://spammer-relay.example.org/very/long/path/blob",
			Host:        "spammer-relay.example.org",
			Online:      false,
			Blacklisted: true,
			Latency:     "—",
			Source:      "manual",
			LastCheck:   "10m ago",
		},
	}
	if err := pages.Relays(rec, PageData{Title: "insomniaXXX", Relays: relays}); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile("/tmp/verify_relays.html", rec.Body.Bytes(), 0644)
}

func TestErrorPageRendering(t *testing.T) {
	pages, err := NewPages()
	if err != nil {
		t.Fatal(err)
	}

	// 1. Test 404 defaults
	rec404 := httptest.NewRecorder()
	if err := pages.Error(rec404, PageData{StatusCode: 404}); err != nil {
		t.Fatalf("pages.Error 404 failed: %v", err)
	}
	body404 := rec404.Body.String()
	for _, needle := range []string{
		"404",
		"Not Found",
		"The requested file or page does not exist",
		`class="error-card"`,
		`class="logo-box"`,
		`href="/"`,
		`href="/relays"`,
	} {
		if !strings.Contains(body404, needle) {
			t.Fatalf("error 404 page missing %q", needle)
		}
	}

	// 2. Test 502 with custom message and details
	rec502 := httptest.NewRecorder()
	if err := pages.Error(rec502, PageData{
		StatusCode:   502,
		ErrorTitle:   "Storage Down",
		ErrorMessage: "All configured Blossom relays are unreachable.",
		ErrorDetail:  "dial tcp 127.0.0.1: Connection refused",
	}); err != nil {
		t.Fatalf("pages.Error 502 failed: %v", err)
	}
	body502 := rec502.Body.String()
	for _, needle := range []string{
		"502",
		"Storage Down",
		"All configured Blossom relays are unreachable",
		"dial tcp 127.0.0.1: Connection refused",
		`class="error-detail-box"`,
	} {
		if !strings.Contains(body502, needle) {
			t.Fatalf("error 502 page missing %q", needle)
		}
	}

	_ = os.WriteFile("/tmp/verify_error.html", rec404.Body.Bytes(), 0644)
}

func TestUploadButtonLoadingNeedsExplicitState(t *testing.T) {
	if strings.Contains(PicoThemeCSS, ".btn-action:disabled .btn-loading") {
		t.Fatal("disabled upload button must not display a loading indicator")
	}
	for _, selector := range []string{
		"#submitBtn.is-loading .btn-text",
		"#submitBtn.is-loading .btn-loading",
		"#submitBtn .btn-loading",
	} {
		if !strings.Contains(PicoThemeCSS, selector) {
			t.Fatalf("upload CSS missing %q", selector)
		}
	}
	if strings.Contains(PicoThemeCSS, "#submitBtn:focus .btn-loading") {
		t.Fatal("keyboard focus must not trigger the upload loading indicator")
	}
}

func TestZKDownloadProxyOptionsBelowPreview(t *testing.T) {
	pages, err := NewPages()
	if err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	if err := pages.View(rec, PageData{
		Title:     "insomniaXXX",
		ZK:        true,
		CipherURL: "/d/example?cipher=1",
	}); err != nil {
		t.Fatal(err)
	}
	body := rec.Body.String()
	positions := []struct {
		name string
		html string
	}{
		{"revealed content", `id="zkContent"`},
		{"file preview", `id="previewBox"`},
		{"proxy option", `id="zkProxyOptions"`},
		{"download buttons", `<div class="grid">`},
	}
	prev := -1
	for _, part := range positions {
		pos := strings.Index(body, part.html)
		if pos <= prev {
			t.Fatalf("%s must appear after the preceding preview element", part.name)
		}
		prev = pos
	}
	if strings.Count(body, `id="proxyOnly"`) != 1 {
		t.Fatal("Zero-Knowledge download must render exactly one proxy checkbox")
	}

	// Server-side and onion views must not show the browser-only proxy toggle.
	for _, data := range []PageData{
		{Title: "insomniaXXX"},
		{Title: "insomniaXXX", ZK: true, ServerMode: true},
	} {
		other := httptest.NewRecorder()
		if err := pages.View(other, data); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(other.Body.String(), `id="proxyOnly"`) {
			t.Fatal("browser-only proxy checkbox appeared in another download mode")
		}
	}
}

func TestNoJSTorPrivacyNoticeRecommendsWASM(t *testing.T) {
	pages, err := NewPages()
	if err != nil {
		t.Fatal(err)
	}

	onion := httptest.NewRecorder()
	if err := pages.Home(onion, PageData{Title: "insomniaXXX", ServerMode: true}); err != nil {
		t.Fatal(err)
	}
	body := onion.Body.String()
	start := strings.Index(body, `<small id="wasmStatus" class="muted">`)
	if start < 0 {
		t.Fatal("No-JS status notice is missing")
	}
	end := strings.Index(body[start:], "</small>")
	if end < 0 {
		t.Fatal("No-JS status notice is not closed")
	}
	notice := body[start : start+end]
	for _, required := range []string{
		"Tor / No-JS works without JavaScript.",
		"Files are encrypted before reaching Blossom",
		"this server handles the file and its key",
		"we recommend Zero-Knowledge (WASM) when available",
	} {
		if !strings.Contains(notice, required) {
			t.Fatalf("No-JS Tor notice missing %q", required)
		}
	}
	if len(notice) > 260 || strings.Contains(notice, "Privacy warning") {
		t.Fatal("No-JS Tor message must stay concise and reassuring")
	}
	if strings.Contains(body, "/static/boot.js") {
		t.Fatal("No-JS Tor notice must work without JavaScript")
	}

	clearnet := httptest.NewRecorder()
	if err := pages.Home(clearnet, PageData{Title: "insomniaXXX"}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(clearnet.Body.String(), "Tor / No-JS works without JavaScript.") {
		t.Fatal("server-side encryption notice must appear only in No-JS Tor mode")
	}
}

func TestNoJSNativeUploadKeepsLoadingAnimationWithoutJavaScript(t *testing.T) {
	pages, err := NewPages()
	if err != nil {
		t.Fatal(err)
	}

	onion := httptest.NewRecorder()
	if err := pages.Home(onion, PageData{Title: "insomniaXXX", ServerMode: true}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(onion.Body.String(), `type="submit" id="submitBtn" class="btn-action nojs-upload"`) {
		t.Fatal("onion upload must use the native No-JS loading CSS")
	}
	if strings.Contains(onion.Body.String(), "/static/boot.js") {
		t.Fatal("No-JS upload must not depend on the WebAssembly script")
	}

	clearnet := httptest.NewRecorder()
	if err := pages.Home(clearnet, PageData{Title: "insomniaXXX"}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(clearnet.Body.String(), `id="submitBtn" class="btn-action nojs-upload"`) {
		t.Fatal("ZK upload must keep its JavaScript-controlled loading state")
	}

	for _, css := range []string{
		"#submitBtn.nojs-upload .btn-text,",
		"#submitBtn.nojs-upload .btn-loading {",
		"transition: opacity 0s linear 999s, visibility 0s linear 999s;",
		"#file:valid ~ #submitBtn.nojs-upload:active .btn-text {",
		"#file:valid ~ #submitBtn.nojs-upload:active .btn-loading {",
		"transition-delay: 0s, 0s !important;",
	} {
		if !strings.Contains(PicoThemeCSS, css) {
			t.Fatalf("No-JS upload CSS must preserve the spinner after click: %q", css)
		}
	}
	if strings.Contains(PicoThemeCSS, "#submitBtn.nojs-upload:focus") {
		t.Fatal("keyboard focus alone must not start the upload animation")
	}
}

func TestNoJSButtonLoadingStates(t *testing.T) {
	pages, err := NewPages()
	if err != nil {
		t.Fatal(err)
	}

	// 1. Home page: upload button
	recHome := httptest.NewRecorder()
	if err := pages.Home(recHome, PageData{Title: "insomniaXXX"}); err != nil {
		t.Fatal(err)
	}
	bodyHome := recHome.Body.String()
	for _, needle := range []string{
		`class="btn-action"`,
		`class="btn-text">Upload</span>`,
		`class="btn-loading"`,
		`btn-spinner`,
		`Uploading &amp; Encrypting...`,
		`class="upload-btn-mascot"`,
		`/static/xxx.png`,
	} {
		if !strings.Contains(bodyHome, needle) {
			t.Fatalf("home page missing %q", needle)
		}
	}

	// 2. View page (No-JS / ZK: false): download & decrypt button
	recViewNoJS := httptest.NewRecorder()
	if err := pages.View(recViewNoJS, PageData{
		Title:       "insomniaXXX",
		ZK:          false,
		FileName:    "test.txt",
		FileSize:    "42 KB",
		DownloadURL: "/d/hash/key?download=1",
	}); err != nil {
		t.Fatal(err)
	}
	bodyView := recViewNoJS.Body.String()
	for _, needle := range []string{
		`class="btn-action"`,
		`class="btn-text">Download (42 KB)</span>`,
		`class="btn-loading"`,
		`btn-spinner`,
		`Decrypting &amp; Downloading...`,
	} {
		if !strings.Contains(bodyView, needle) {
			t.Fatalf("view page (no-js) missing %q", needle)
		}
	}
	_ = os.WriteFile("/tmp/verify_view_nojs.html", recViewNoJS.Body.Bytes(), 0644)

	// 3. Done page: download / decrypt link
	recDone := httptest.NewRecorder()
	if err := pages.Done(recDone, PageData{
		Title:    "insomniaXXX",
		ShareURL: "/d/hash/key",
		FileName: "test.txt",
		FileSize: "42 KB",
	}); err != nil {
		t.Fatal(err)
	}
	bodyDone := recDone.Body.String()
	for _, needle := range []string{
		`class="btn-action"`,
		`class="btn-text">Download / Preview</span>`,
		`class="btn-loading"`,
		`btn-spinner`,
		`Decrypting &amp; Loading...`,
	} {
		if !strings.Contains(bodyDone, needle) {
			t.Fatalf("done page missing %q", needle)
		}
	}
}
