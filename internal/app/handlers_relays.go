package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"anonblossom/internal/web"
)

func (a *Server) handleRelays(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Has("json") || strings.Contains(r.Header.Get("Accept"), "application/json") {
		urls := a.storage.GetActiveURLs()
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(urls)
		return
	}

	servers := a.storage.GetAllServers()
	viewList := make([]web.RelayView, 0, len(servers))

	for _, s := range servers {
		latencyStr := "—"
		if s.Online && s.Latency > 0 {
			latencyStr = fmt.Sprintf("%d ms", s.Latency.Milliseconds())
		}

		lastCheckStr := "Never"
		if !s.LastChecked.IsZero() {
			lastCheckStr = time.Since(s.LastChecked).Truncate(time.Second).String() + " ago"
		}

		viewList = append(viewList, web.RelayView{
			URL:         s.URL,
			Host:        s.Host,
			Online:      s.Online,
			Blacklisted: s.Blacklisted,
			Latency:     latencyStr,
			Source:      s.Source,
			LastCheck:   lastCheckStr,
		})
	}

	w.Header().Set("Cache-Control", "no-store")
	_ = a.pages.Relays(w, web.PageData{
		Title:  pageTitle,
		Relays: viewList,
	})
}
