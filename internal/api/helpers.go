package api

import (
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"strings"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func spaHandler(root fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(root))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}
		isAsset := strings.HasPrefix(path, "assets/")
		f, err := root.Open(path)
		if err != nil {
			// A chunk from a previous build: answering with index.html would
			// break the module loader with a MIME error instead of a clean 404.
			if isAsset {
				http.NotFound(w, r)
				return
			}
			index, err := root.Open("index.html")
			if err != nil {
				http.NotFound(w, r)
				return
			}
			defer index.Close()
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-cache")
			_, _ = io.Copy(w, index)
			return
		}
		_ = f.Close()
		switch {
		case isAsset:
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		case path == "index.html":
			// index.html references hashed chunks; it must be revalidated after every deploy.
			w.Header().Set("Cache-Control", "no-cache")
		}
		fileServer.ServeHTTP(w, r)
	})
}
