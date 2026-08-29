package api

import (
	"context"
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"strings"

	"cogitor-iot-platform/internal/models"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func withUser(ctx context.Context, u models.User) context.Context {
	return context.WithValue(ctx, userKey, u)
}

func userFrom(r *http.Request) *models.User {
	v, _ := r.Context().Value(userKey).(models.User)
	if v.ID == 0 {
		return nil
	}
	return &v
}

func spaHandler(root fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(root))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}
		f, err := root.Open(path)
		if err != nil {
			// SPA fallback
			index, err := root.Open("index.html")
			if err != nil {
				http.NotFound(w, r)
				return
			}
			defer index.Close()
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = io.Copy(w, index)
			return
		}
		_ = f.Close()
		fileServer.ServeHTTP(w, r)
	})
}
