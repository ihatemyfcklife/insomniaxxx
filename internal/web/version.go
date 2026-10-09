package web

// Version est surchargée au build via :
// go build -ldflags "-X anonblossom/internal/web.Version=v1.2.3" ./cmd/anonblossom
// Le workflow release.yml l'injecte automatiquement depuis le tag Git.
var Version = "dev"

// AppVersion retourne la version à afficher dans le footer.
func AppVersion() string {
	if Version == "" {
		return "dev"
	}
	return Version
}