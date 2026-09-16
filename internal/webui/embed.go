package webui

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed assets/*
var embedded embed.FS

func Handler() http.Handler {
	root, err := fs.Sub(embedded, "assets")
	if err != nil {
		panic(err)
	}
	files := http.FileServer(http.FS(root))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clean := path.Clean(r.URL.Path)
		if clean == "." || clean == "/" || clean == "/kiosk" {
			data, err := fs.ReadFile(root, "index.html")
			if err != nil {
				http.Error(w, "UI unavailable", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
			_, _ = w.Write(data)
			return
		}
		if strings.HasPrefix(clean, "/api/") {
			http.NotFound(w, r)
			return
		}
		files.ServeHTTP(w, r)
	})
}
