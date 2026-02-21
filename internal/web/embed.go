package web

import (
	"embed"
	"io/fs"
	"net/http"
	"os"
	"path"
)

//go:embed static/*
var staticFiles embed.FS

// Handler returns an http.Handler that serves the embedded SvelteKit SPA.
// For any path that doesn't match a static file, it serves index.html (SPA fallback).
func Handler() http.Handler {
	sub, _ := fs.Sub(staticFiles, "static")
	fileServer := http.FileServer(http.FS(sub))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Clean the path
		p := path.Clean(r.URL.Path)
		if p == "/" {
			p = "/index.html"
		}

		// Try to open the file in the embedded FS
		f, err := sub.Open(p[1:]) // strip leading /
		if err != nil {
			if os.IsNotExist(err) {
				// SPA fallback: serve index.html
				r.URL.Path = "/"
				fileServer.ServeHTTP(w, r)
				return
			}
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		_ = f.Close()

		fileServer.ServeHTTP(w, r)
	})
}
