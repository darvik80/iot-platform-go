package api

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"

	"cogitor-iot-platform/internal/config"
	"cogitor-iot-platform/internal/db"
	"cogitor-iot-platform/internal/models"
	"cogitor-iot-platform/internal/services"
	"cogitor-iot-platform/internal/ws"
)

type testEnv struct {
	t      *testing.T
	srv    *Server
	h      http.Handler
	plat   *services.Platform
	tokens map[string]string
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	database, err := db.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	var cfg config.Config
	cfg.DataDir = dir
	cfg.Database.Driver = "sqlite"
	cfg.Database.DSN = dbPath
	cfg.Auth.JWTSecret = "test-secret-0123456789abcdef"
	cfg.Register.Enabled = true
	cfg.Register.DefaultRole = models.RoleAdmin
	cfg.Rules.Prefix = "platform/v1"

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	plat := services.New(database, cfg, log)
	srv := &Server{Plat: plat, Hub: ws.NewHub()}
	return &testEnv{t: t, srv: srv, h: srv.Router(), plat: plat, tokens: map[string]string{}}
}

// user registers login (personal account slug = login) and caches a JWT for it.
func (e *testEnv) user(login string) *models.User {
	e.t.Helper()
	u, err := e.plat.Register(services.RegisterInput{
		Login: login, Email: login + "@example.com", Password: "password123",
	})
	if err != nil {
		e.t.Fatalf("register %s: %v", login, err)
	}
	_, tok, err := e.plat.Login(login, "password123")
	if err != nil {
		e.t.Fatalf("login %s: %v", login, err)
	}
	e.tokens[login] = tok
	return u
}

func (e *testEnv) makePlatformAdmin(u *models.User) {
	e.t.Helper()
	if _, err := e.plat.DB.Exec(`INSERT INTO settings(key, value) VALUES(?, ?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`,
		services.PlatformAdminSettingKey, strconv.FormatInt(u.ID, 10)); err != nil {
		e.t.Fatalf("set platform admin: %v", err)
	}
}

func (e *testEnv) accountID(login string) int64 {
	e.t.Helper()
	var id int64
	if err := e.plat.DB.QueryRow(`SELECT id FROM accounts WHERE slug=?`, login).Scan(&id); err != nil {
		e.t.Fatalf("account %s: %v", login, err)
	}
	return id
}

func (e *testEnv) device(login, product, device string) *models.Device {
	e.t.Helper()
	acc := e.accountID(login)
	if _, err := e.plat.GetProductBySlugInAccount(acc, product); err != nil {
		if _, err := e.plat.CreateProduct(acc, services.ProductInput{ProductID: product, Name: product}); err != nil {
			e.t.Fatalf("create product %s/%s: %v", login, product, err)
		}
	}
	d, _, err := e.plat.CreateDevice(acc, services.DeviceInput{ProductID: product, DeviceID: device})
	if err != nil {
		e.t.Fatalf("create device %s/%s.%s: %v", login, product, device, err)
	}
	return d
}

func (e *testEnv) do(method, path, as string, body any) *httptest.ResponseRecorder {
	e.t.Helper()
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			e.t.Fatal(err)
		}
		rd = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, rd)
	req.Header.Set("Content-Type", "application/json")
	if as != "" {
		req.Header.Set("Authorization", "Bearer "+e.tokens[as])
	}
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	return rec
}

func (e *testEnv) expect(rec *httptest.ResponseRecorder, want int, what string) {
	e.t.Helper()
	if rec.Code != want {
		e.t.Errorf("%s: status %d, want %d; body=%s", what, rec.Code, want, rec.Body.String())
	}
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
	return v
}

func itoa(id int64) string { return strconv.FormatInt(id, 10) }
