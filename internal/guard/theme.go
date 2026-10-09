package guard

import (
	"fmt"
	"html"
	"image/color"
	"net/http"
	"strings"
	"time"

	"anonblossom/internal/web"
	onionguard "github.com/ihatemyfcklife/onionguard"
)

const pageTitle = "insomniaXXX"

func Configure(maxBodyBytes int64, cookieSecure bool) onionguard.Config {
	cfg := onionguard.DefaultConfig()
	cfg.WaitRoom.Enabled = true
	cfg.WaitRoom.WaitTime = 5 * time.Second
	cfg.Captcha.Enabled = true
	cfg.Captcha.Length = 5
	cfg.Captcha.Visual = onionguard.CaptchaVisualConfig{
		BackgroundColor: mustColor("#181C25"),
		TextColor:       mustColor("#D9269D"),
		LineColor:       mustColor("#525F7A"),
		NoiseLines:      2,
		NoiseRatio:      0.02,
		JitterPixels:    2,
	}
	cfg.SecurityHeaders.Policy = onionguard.HeaderPreserve
	cfg.SecurityHeaders.ContentSecurityPolicy = web.PicoCSP
	cfg.CustomWaitRoomHTML = waitRoomHTML
	cfg.CustomChallengeHTML = challengeHTML
	cfg.CustomErrorHTML = errorHTML
	cfg.CustomSuccessHTML = successHTML
	cfg.MaxBodyBytes = maxBodyBytes
	cfg.CookieSecure = cookieSecure
	return cfg
}

func waitRoomHTML(_ *http.Request, retry time.Duration) string {
	seconds := max(1, int(retry.Seconds()))
	return fmt.Sprintf(`<!doctype html>
<html lang="en" data-theme="dark">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <meta http-equiv="refresh" content="%d">
  <meta name="referrer" content="no-referrer">
  <title>Checking · %s</title>
  <link rel="icon" type="image/png" href="/static/favicon.png">
  <link rel="icon" type="image/x-icon" href="/favicon.ico">
  <link rel="stylesheet" href="/static/pico.min.css">
  <style>%s</style>
</head>
<body>
  <main class="container">
    %s
    <article>
      <h1>Checking</h1>
      <p>Please wait %d seconds...</p>
      <small class="muted">This page refreshes automatically. Powered by <a href="https://github.com/ihatemyfcklife/onionguard" class="view-relays-link">Onionguard</a>, No JavaScript required.</small>
    </article>
    %s
  </main>
</body>
</html>`, seconds, pageTitle, web.PicoThemeCSS, web.LogoHTML(), seconds, web.FooterHTML())
}

func challengeHTML(r *http.Request, challenge *onionguard.CaptchaChallenge, cfg onionguard.CaptchaConfig) string {
	target := "/"
	if r != nil {
		if r.URL != nil && r.URL.Path != cfg.EndpointPath {
			// Premier affichage : on garde l'URL d'origine demandée (/d/...).
			// Reproduit la logique du middleware OnionGuard qui utilise RequestURI.
			raw := r.URL.RequestURI()
			if raw == "" {
				raw = "/"
			}
			target = sanitizeChallengeTarget(raw)
		} else {
			// Ré-affichage depuis l'endpoint (erreur de saisie) : reprendre le
			// champ du formulaire, `r` en priorité comme le middleware.
			raw := r.FormValue("r")
			if raw == "" {
				raw = r.FormValue("target")
			}
			if raw == "" && r.URL != nil {
				raw = r.URL.Query().Get("r")
			}
			if raw == "" && r.URL != nil {
				raw = r.URL.Query().Get("target")
			}
			target = sanitizeChallengeTarget(raw)
		}
	}

	return fmt.Sprintf(`<!doctype html>
<html lang="en" data-theme="dark">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <meta name="referrer" content="no-referrer">
  <title>Verify · %s</title>
  <link rel="icon" type="image/png" href="/static/favicon.png">
  <link rel="icon" type="image/x-icon" href="/favicon.ico">
  <link rel="stylesheet" href="/static/pico.min.css">
  <style>%s</style>
</head>
<body>
  <main class="container">
    %s
    <article>
      <h1>Verify</h1>
      <p class="muted">One small check before continuing.</p>
      <img src="%s" alt="CAPTCHA challenge" width="%d" height="%d">
      <form action="%s" method="post">
        <input type="hidden" name="r" value="%s">
        <input type="hidden" name="target" value="%s">
		<br>
        <input id="answer" name="answer" type="text" inputmode="text" autocomplete="off" autocapitalize="characters" spellcheck="false" maxlength="%d" placeholder="Captcha Code" required>
        <button type="submit">Continue</button>
      </form>
      <small class="muted">Powered by <a href="https://github.com/ihatemyfcklife/onionguard" class="view-relays-link">Onionguard</a>, No JavaScript required.</small>
    </article>
    %s
  </main>
</body>
</html>`, pageTitle, web.PicoThemeCSS, web.LogoHTML(), html.EscapeString(challenge.ImageDataURI), cfg.Width, cfg.Height, html.EscapeString(cfg.EndpointPath), html.EscapeString(target), html.EscapeString(target), cfg.Length, web.FooterHTML())
}

// sanitizeChallengeTarget reprend les règles du middleware OnionGuard :
// chemin absolu interne uniquement, pas de //, pas de backslash,
// pas de caractères de contrôle (anti open-redirect / header injection).
func sanitizeChallengeTarget(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "/"
	}
	if !strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "//") {
		return "/"
	}
	if strings.Contains(raw, "\\") {
		return "/"
	}
	for i := 0; i < len(raw); i++ {
		if raw[i] < 32 || raw[i] == 127 {
			return "/"
		}
	}
	return raw
}

func errorHTML(_ *http.Request, err *onionguard.AdmissionError) string {
	message := "Request blocked."
	if err != nil && err.Message != "" {
		message = err.Message
	}
	return fmt.Sprintf(`<!doctype html>
<html lang="en" data-theme="dark">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <meta name="referrer" content="no-referrer">
  <title>Unavailable · %s</title>
  <link rel="icon" type="image/png" href="/static/favicon.png">
  <link rel="icon" type="image/x-icon" href="/favicon.ico">
  <link rel="stylesheet" href="/static/pico.min.css">
  <style>%s</style>
</head>
<body>
  <main class="container">
    %s
    <article role="alert">
      <h1>Unavailable</h1>
      <p>%s</p>
      <a href="/" class="view-relays-link">Back</a>
    </article>
    %s
  </main>
</body>
</html>`, pageTitle, web.PicoThemeCSS, web.LogoHTML(), html.EscapeString(message), web.FooterHTML())
}

func successHTML(_ *http.Request, target string) string {
	if target == "" || !strings.HasPrefix(target, "/") || strings.HasPrefix(target, "//") || strings.Contains(target, "\\") {
		target = "/"
	}
	escapedTarget := html.EscapeString(target)

	return fmt.Sprintf(`<!doctype html>
<html lang="en" data-theme="dark">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <meta http-equiv="refresh" content="1;url=%s">
  <meta name="referrer" content="no-referrer">
  <title>Passed · %s</title>
  <link rel="icon" type="image/png" href="/static/favicon.png">
  <link rel="icon" type="image/x-icon" href="/favicon.ico">
  <link rel="stylesheet" href="/static/pico.min.css">
  <style>%s</style>
</head>
<body>
  <main class="container">
    %s
    <article role="status">
      <h1>Passed</h1>
      <p>Verification passed. Redirecting...</p>
      <a href="%s" class="view-relays-link">Continue</a>
    </article>
    %s
  </main>
</body>
</html>`, escapedTarget, pageTitle, web.PicoThemeCSS, web.LogoHTML(), escapedTarget, web.FooterHTML())
}

func mustColor(hexValue string) color.RGBA {
	var r, g, b uint8
	_, _ = fmt.Sscanf(hexValue, "#%02x%02x%02x", &r, &g, &b)
	return color.RGBA{R: r, G: g, B: b, A: 255}
}
