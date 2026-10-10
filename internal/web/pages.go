package web

import (
	"bytes"
	"embed"
	"html/template"
	"net/http"
)

//go:embed templates/*.html
var templateFS embed.FS

// LogoHTML returns the rendered HTML of the responsive ASCII logo.
func LogoHTML() string {
	t, err := template.ParseFS(templateFS, "templates/*.html")
	if err != nil {
		return ""
	}
	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, "logo", map[string]any{"Version": AppVersion()}); err != nil {
		return ""
	}
	return buf.String()
}

// FooterHTML returns the rendered HTML of the site footer, with the current
// build version. Rendered on each call (no cache) so APP_VERSION or test
// overrides are always reflected.
func FooterHTML() string {
	t, err := template.ParseFS(templateFS, "templates/*.html")
	if err != nil {
		return ""
	}
	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, "footer", map[string]any{"Version": AppVersion()}); err != nil {
		return ""
	}
	return buf.String()
}

type RelayView struct {
	URL         string
	Host        string
	Online      bool
	Blacklisted bool
	Latency     string
	Source      string
	LastCheck   string
}

// PageData is the data rendered into the public pages.
type PageData struct {
	Title          string
	Version        string
	Error          string
	StatusCode     int
	ErrorTitle     string
	ErrorMessage   string
	ErrorDetail    string
	ShareURL       string
	RawURL         string
	DownloadURL    string
	ZK             bool
	CipherURL      string
	FileName       string
	FileSize       string
	FileType       string
	IsImage        bool
	IsVideo        bool
	IsAudio        bool
	MaxUpload      string
	MaxUploadBytes int64
	BlobHash       string
	Key            string
	QRCode         template.HTML
	ThemeCSS       template.CSS
	Relays         []RelayView
}

// withVersion injecte la version binaire si absente (rendu 100% serveur, sans JS).
func withVersion(data PageData) PageData {
	if data.Version == "" {
		data.Version = AppVersion()
	}
	return data
}

// Pages renders the embedded server-side HTML templates.
type Pages struct {
	home   *template.Template
	done   *template.Template
	view   *template.Template
	relays *template.Template
	err    *template.Template
}

// NewPages parses the embedded HTML templates.
func NewPages() (*Pages, error) {
	templates, err := template.ParseFS(templateFS, "templates/*.html")
	if err != nil {
		return nil, err
	}
	return &Pages{
		home:   templates.Lookup("home.html"),
		done:   templates.Lookup("done.html"),
		view:   templates.Lookup("view.html"),
		relays: templates.Lookup("relays.html"),
		err:    templates.Lookup("error.html"),
	}, nil
}

func (p *Pages) Home(w http.ResponseWriter, data PageData) error {
	data = withVersion(data)
	if data.ThemeCSS == "" {
		data.ThemeCSS = template.CSS(PicoThemeCSS)
	}
	return p.home.Execute(w, data)
}

func (p *Pages) Done(w http.ResponseWriter, data PageData) error {
	data = withVersion(data)
	if data.ThemeCSS == "" {
		data.ThemeCSS = template.CSS(PicoThemeCSS)
	}
	return p.done.Execute(w, data)
}

func (p *Pages) View(w http.ResponseWriter, data PageData) error {
	data = withVersion(data)
	if data.ThemeCSS == "" {
		data.ThemeCSS = template.CSS(PicoThemeCSS)
	}
	return p.view.Execute(w, data)
}

func (p *Pages) Relays(w http.ResponseWriter, data PageData) error {
	data = withVersion(data)
	if data.ThemeCSS == "" {
		data.ThemeCSS = template.CSS(PicoThemeCSS)
	}
	return p.relays.Execute(w, data)
}

func (p *Pages) Error(w http.ResponseWriter, data PageData) error {
	data = withVersion(data)
	if data.StatusCode == 0 {
		data.StatusCode = http.StatusNotFound
	}
	if data.ErrorTitle == "" {
		data.ErrorTitle = http.StatusText(data.StatusCode)
		if data.ErrorTitle == "" {
			data.ErrorTitle = "Error"
		}
	}
	if data.ErrorMessage == "" {
		switch data.StatusCode {
		case http.StatusNotFound:
			data.ErrorMessage = "The requested file or page does not exist, has expired, or the link is invalid."
		case http.StatusBadGateway:
			data.ErrorMessage = "The remote Blossom storage relays could not be reached or are temporarily unavailable."
		case http.StatusServiceUnavailable:
			data.ErrorMessage = "The service is temporarily unavailable. Please try again shortly."
		case http.StatusBadRequest:
			data.ErrorMessage = "The request could not be processed due to invalid parameters."
		case http.StatusInternalServerError:
			data.ErrorMessage = "An unexpected server error occurred."
		default:
			data.ErrorMessage = "An error occurred while processing your request."
		}
	}
	if data.Title == "" {
		data.Title = "insomniaXXX"
	}
	if data.ThemeCSS == "" {
		data.ThemeCSS = template.CSS(PicoThemeCSS)
	}
	return p.err.Execute(w, data)
}
