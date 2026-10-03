package web

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed all:dist
var distFS embed.FS

// Handler serves static assets from Vite's dist and handles SPA client routing fallback.
func Handler() http.HandlerFunc {
	dist, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic(err)
	}

	fileServer := http.FileServer(http.FS(dist))

	return func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")

		// Check if a static file exists (e.g. assets/*.js, assets/*.css, favicon.svg)
		if path != "" {
			if f, err := dist.Open(path); err == nil {
				_ = f.Close()
				fileServer.ServeHTTP(w, r)
				return
			}
		}

		// Fallback for SPA routing: serve index.html
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	}
}
