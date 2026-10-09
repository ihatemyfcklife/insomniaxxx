package web

import (
	"os"
	"runtime/debug"
	"strings"
)

// Version est surchargée au build via :
// go build -ldflags "-X anonblossom/internal/web.Version=v1.2.3" ./cmd/anonblossom
// Le workflow release.yml l'injecte automatiquement depuis le tag Git.
var Version = ""

// AppVersion retourne la version à afficher dans le footer.
// Ordre : ldflags > APP_VERSION (env) > go build info (VCS) > dev.
func AppVersion() string {
	if v := strings.TrimSpace(Version); v != "" && v != "dev" {
		return v
	}
	if v := strings.TrimSpace(os.Getenv("APP_VERSION")); v != "" {
		return v
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, s := range info.Settings {
			if s.Key == "vcs.tag" && strings.TrimSpace(s.Value) != "" {
				return s.Value
			}
		}
		if info.Main.Version != "" && info.Main.Version != "(devel)" {
			return info.Main.Version
		}
	}
	if strings.TrimSpace(Version) != "" {
		return Version
	}
	return "dev"
}