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
