package api

import (
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func TestSPAHandlerCaching(t *testing.T) {
	h := spaHandler(fstest.MapFS{
		"index.html":      {Data: []byte("<html></html>")},
		"assets/app-1.js": {Data: []byte("console.log(1)")},
	})
	get := func(path string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		return rec
	}

	for _, path := range []string{"/", "/products/5"} {
		rec := get(path)
		if rec.Code != 200 || rec.Header().Get("Cache-Control") != "no-cache" {
			t.Errorf("%s: code=%d cache=%q, want 200 no-cache", path, rec.Code, rec.Header().Get("Cache-Control"))
		}
	}
	if rec := get("/assets/app-1.js"); rec.Code != 200 || rec.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" {
		t.Errorf("asset: code=%d cache=%q", rec.Code, rec.Header().Get("Cache-Control"))
	}
	if rec := get("/assets/UsersPage-old.js"); rec.Code != 404 {
		t.Errorf("stale chunk: code=%d, want 404", rec.Code)
	}
}
