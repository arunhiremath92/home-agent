package webui

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed static/*
var files embed.FS

func Handler() http.Handler {
	content, err := fs.Sub(files, "static")
	if err != nil {
		panic(err)
	}
	fileServer := http.FileServer(http.FS(content))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || (r.URL.Path != "/" &&
			r.URL.Path != "/app.css" && r.URL.Path != "/app.js") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; connect-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'")
		fileServer.ServeHTTP(w, r)
	})
}
