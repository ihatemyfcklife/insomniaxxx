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
			if !strings.Contains(tc.html, `class="logo-box"`) {
				t.Fatalf("%s does not contain class=\"logo-box\"", tc.name)
			}
			if !strings.Contains(tc.html, `class="site-logo"`) {
				t.Fatalf("%s does not contain class=\"site-logo\"", tc.name)
			}
			if !strings.Contains(tc.html, `/static/logo.png`) {
				t.Fatalf("%s does not contain /static/logo.png", tc.name)
			}
			if !strings.Contains(tc.html, `Anonymous file sharing. No accounts. No logs. Decentralized.`) {
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

func TestPoweredRowRender(t *testing.T) {
	req := httptest.NewRequest("GET", "http://example.com/test", nil)

	// Page wait-room (attente des secondes) : zzz.png + powered-row + lien onionguard.
	waitRoom := waitRoomHTML(req, 5*time.Second)
	for _, needle := range []string{"/static/zzz.png", `class="powered-row"`, `Powered by <a href="https://github.com/ihatemyfcklife/onionguard"`} {
		if !strings.Contains(waitRoom, needle) {
			t.Fatalf("waitRoomHTML missing %q: %s", needle, waitRoom)
		}
	}

	// Page d'erreur : zzz.png + powered-row.
	err := errorHTML(req, &onionguard.AdmissionError{Message: "Too many requests"})
	for _, needle := range []string{"/static/zzz.png", `class="powered-row"`} {
		if !strings.Contains(err, needle) {
			t.Fatalf("errorHTML missing %q: %s", needle, err)
		}
	}

	// Page de captcha : zzz.png NE doit pas être là.
	challenge := challengeHTML(req, &onionguard.CaptchaChallenge{
		ImageDataURI: "data:image/svg+xml;utf8,<svg xmlns='http://www.w3.org/2000/svg' width='200' height='80'></svg>",
	}, onionguard.CaptchaConfig{
		Width:        200,
		Height:       80,
		EndpointPath: "/captcha",
		Length:       5,
	})
	if strings.Contains(challenge, "/static/zzz.png") {
		t.Fatalf("challengeHTML should not contain zzz.png: %s", challenge)
	}
}

