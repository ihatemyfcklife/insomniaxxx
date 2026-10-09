package qr

import (
	"fmt"
	"html"
	"html/template"
	"image/color"
	"net/http"
	"net/url"

	qrcode "github.com/skip2/go-qrcode"
)

// Size est la largeur et hauteur en pixels du PNG généré.
const Size = 256

// Handler sert le code QR au format PNG avec les couleurs du thème sombre.
func Handler(w http.ResponseWriter, r *http.Request) {
	data := r.URL.Query().Get("data")
	if data == "" || len(data) > 512 {
		http.Error(w, "invalid QR data", http.StatusBadRequest)
		return
	}

	code, err := qrcode.New(data, qrcode.Medium)
	if err != nil {
		http.Error(w, "could not generate QR code", http.StatusInternalServerError)
		return
	}

	// Couleurs assorties à l'interface sombre
	code.BackgroundColor = color.RGBA{R: 0x18, G: 0x1C, B: 0x25, A: 0xFF} // #181C25
	code.ForegroundColor = color.RGBA{R: 0xC2, G: 0xC7, B: 0xD0, A: 0xFF} // #C2C7D0

	png, err := code.PNG(Size)
	if err != nil {
		http.Error(w, "could not encode PNG", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	_, _ = w.Write(png)
}

// Img renvoie la balise <img> pointant vers l'endpoint PNG.
func Img(content string) template.HTML {
	src := html.EscapeString("/qr?data=" + url.QueryEscape(content))
	return template.HTML(fmt.Sprintf(`<img src="%s" alt="QR code" width="%d" height="%d">`, src, Size, Size))
}
