package guard

import (
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	onionguard "github.com/ihatemyfcklife/onionguard"
)

func TestGuardPagesRenderLogo(t *testing.T) {
	req := httptest.NewRequest("GET", "http://example.com/test", nil)

	testCases := []struct {
		name string
		html string
	}{
		{
			name: "waitRoom",
			html: waitRoomHTML(req, 5*time.Second),
		},
		{
			name: "challenge",
			html: challengeHTML(req, &onionguard.CaptchaChallenge{
				ImageDataURI: "data:image/png;base64,abc",
			}, onionguard.CaptchaConfig{
				Width:        200,
				Height:       80,
				EndpointPath: "/captcha",
				Length:       5,
			}),
		},
		{
			name: "error",
			html: errorHTML(req, &onionguard.AdmissionError{Message: "Too many requests"}),
		},
		{
			name: "success",
			html: successHTML(req, "/continue"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if !strings.Contains(tc.html, `/static/zk-fragment.js`) {
				t.Fatalf("%s does not load key-continuity script", tc.name)
			}
			if !strings.Contains(tc.html, `class="logo-box"`) {
				t.Fatalf("%s does not contain class=\"logo-box\"", tc.name)
			}
			if !strings.Contains(tc.html, `class="site-logo"`) {
				t.Fatalf("%s does not contain class=\"site-logo\"", tc.name)
			}
			if !strings.Contains(tc.html, `/static/logo.png`) {
				t.Fatalf("%s does not contain /static/logo.png", tc.name)
			}
			if !strings.Contains(tc.html, `File sharing. No accounts. Decentralized relays.`) {
				t.Fatalf("%s does not contain tagline", tc.name)
			}
			if !strings.Contains(tc.html, `<header>`) {
				t.Fatalf("%s does not contain <header>", tc.name)
			}
			if !strings.Contains(tc.html, `class="site-footer"`) {
				t.Fatalf("%s does not contain class=\"site-footer\"", tc.name)
			}
		})
	}
}

func TestWaitRoomRetriesAfterFiveSecondsWithoutJavaScript(t *testing.T) {
	for _, tc := range []struct {
		name string
		url string
		delay time.Duration
		refresh string
		link string
	}{
		{"plain", "http://example.com/", 5 * time.Second, `content="5"`, `href="/"`},
		{"fractional-delay", "http://example.com/d/abc?cipher=1&raw=1", 4500 * time.Millisecond, `content="5"`, `href="/d/abc?cipher=1&amp;raw=1"`},
		{"at-least-one-second", "http://example.com/d/abc", 300 * time.Millisecond, `content="1"`, `href="/d/abc"`},
		{"no-open-redirect", "http://example.com//evil.example/", 5 * time.Second, `content="5"`, `href="/"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", tc.url, nil)
			page := waitRoomHTML(r, tc.delay)
			for _, required := range []string{
				`<meta http-equiv="refresh" ` + tc.refresh + `>`,
				`<script defer src="/static/guard-wait.js"></script>`,
				`<script defer src="/static/zk-fragment.js"></script>`,
				`continue manually</a>`,
				tc.link,
			} {
				if !strings.Contains(page, required) {
					t.Fatalf("wait room missing %q", required)
				}
			}
			if strings.Index(page, "/static/zk-fragment.js") > strings.Index(page, "/static/guard-wait.js") {
				t.Fatal("ZK key must be stashed before automatic navigation starts")
			}
		})
	}
	if !strings.Contains(waitRoomHTML(nil, 5*time.Second), `href="/"`) {
		t.Fatal("wait room without request must have a safe retry path")
	}
}

func TestSuccessContinueLinkFollowsVerificationText(t *testing.T) {
	page := successHTML(nil, "/d/abc?cipher=1")
	status := strings.Index(page, "Verification passed. Redirecting...")
	powered := strings.Index(page, "Powered by")
	link := strings.Index(page, `<p class="guard-success-action"><a href="/d/abc?cipher=1" class="view-relays-link">Continue</a></p>`)
	if status < 0 || powered <= status || link <= powered {
		t.Fatal("Continue text link must appear on a separate line after all success text")
	}
	if !strings.Contains(page, ".guard-success-action {") {
		t.Fatal("success link spacing must use the shared site theme")
	}
}

func TestDumpChallengePage(t *testing.T) {
	req := httptest.NewRequest("GET", "http://example.com/test", nil)
	c := challengeHTML(req, &onionguard.CaptchaChallenge{
		ImageDataURI: "data:image/svg+xml;utf8,<svg xmlns='http://www.w3.org/2000/svg' width='200' height='80'><rect width='200' height='80' fill='%23181C25'/><text x='40' y='50' fill='%23C1208B' font-size='28'>X9K4B</text></svg>",
	}, onionguard.CaptchaConfig{
		Width:        200,
		Height:       80,
		EndpointPath: "/captcha",
		Length:       5,
	})
	_ = os.WriteFile("/tmp/verify_guard.html", []byte(c), 0644)
}
