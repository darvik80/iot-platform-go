package api

import (
	"net/http"
	"testing"
	"time"

	"cogitor-iot-platform/internal/services"
)

func TestSetupRoutesClosedWhilePlatformRuns(t *testing.T) {
	e := newTestEnv(t)
	e.srv.Setup = services.NewSetupService(nil)
	h := e.srv.Router()

	e.h = h
	rec := e.do("GET", "/api/v1/setup/state", "", nil)
	e.expect(rec, http.StatusOK, "setup state")
	if got := decode[map[string]any](t, rec); got["completed"] != true || len(got) != 1 {
		t.Errorf("setup state = %v, want only {completed:true}", got)
	}
	for _, path := range []string{"/api/v1/setup/step", "/api/v1/setup/apply", "/api/v1/setup/generate-tls"} {
		if rec := e.do("POST", path, "", map[string]any{}); rec.Code == http.StatusOK {
			t.Errorf("POST %s is reachable after setup (status 200)", path)
		}
	}
}

func TestLoginLockedOutAfterFailures(t *testing.T) {
	e := newTestEnv(t)
	e.user("bob")
	bad := map[string]any{"login": "bob", "password": "wrong-password"}
	for i := 0; i < 5; i++ {
		e.expect(e.do("POST", "/api/v1/auth/login", "", bad), http.StatusUnauthorized, "wrong password")
	}
	rec := e.do("POST", "/api/v1/auth/login", "", map[string]any{"login": "BOB", "password": "password123"})
	e.expect(rec, http.StatusTooManyRequests, "correct password while locked out")
	if rec.Header().Get("Retry-After") == "" {
		t.Error("429 without Retry-After")
	}

	e.user("carol")
	e.expect(e.do("POST", "/api/v1/auth/login", "", map[string]any{"login": "carol", "password": "password123"}),
		http.StatusOK, "other identity is not locked")
}

func TestAuthEndpointsRateLimitedPerIP(t *testing.T) {
	e := newTestEnv(t)
	for i := 0; i < 20; i++ {
		e.expect(e.do("GET", "/api/v1/auth/check-slug?slug=free", "", nil), http.StatusOK, "within burst")
	}
	e.expect(e.do("GET", "/api/v1/auth/check-slug?slug=free", "", nil), http.StatusTooManyRequests, "over burst")
}

func TestRateLimiterRefills(t *testing.T) {
	now := time.Unix(0, 0)
	l := newRateLimiter(2, 10*time.Second)
	l.now = func() time.Time { return now }

	if !l.Allow("k") || !l.Allow("k") {
		t.Fatal("burst of 2 rejected")
	}
	if l.Allow("k") {
		t.Fatal("third request allowed")
	}
	if !l.Blocked("k") || l.RetryAfter("k") != 10 {
		t.Fatalf("blocked=%v retryAfter=%d, want true/10", l.Blocked("k"), l.RetryAfter("k"))
	}
	if !l.Allow("other") {
		t.Fatal("keys are not independent")
	}
	now = now.Add(10 * time.Second)
	if !l.Allow("k") {
		t.Fatal("token not refilled after interval")
	}
	now = now.Add(time.Hour)
	l.Allow("x")
	if _, ok := l.buckets["other"]; ok {
		t.Error("idle bucket not swept")
	}
}
