package api

import (
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// rateLimiter is an in-memory token bucket per key (single process only).
type rateLimiter struct {
	mu        sync.Mutex
	perSec    float64
	burst     float64
	buckets   map[string]*bucket
	lastSweep time.Time
	now       func() time.Time
}

type bucket struct {
	tokens float64
	last   time.Time
}

func newRateLimiter(burst int, every time.Duration) *rateLimiter {
	return &rateLimiter{
		perSec:  1 / every.Seconds(),
		burst:   float64(burst),
		buckets: map[string]*bucket{},
		now:     time.Now,
	}
}

func (l *rateLimiter) refill(key string, now time.Time) *bucket {
	b := l.buckets[key]
	if b == nil {
		b = &bucket{tokens: l.burst, last: now}
		l.buckets[key] = b
		return b
	}
	b.tokens = math.Min(l.burst, b.tokens+now.Sub(b.last).Seconds()*l.perSec)
	b.last = now
	return b
}

func (l *rateLimiter) sweep(now time.Time) {
	if now.Sub(l.lastSweep) < time.Minute {
		return
	}
	l.lastSweep = now
	full := time.Duration(l.burst / l.perSec * float64(time.Second))
	for k, b := range l.buckets {
		if now.Sub(b.last) > full {
			delete(l.buckets, k)
		}
	}
}

// Allow consumes one token; false means the caller must back off.
func (l *rateLimiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.sweep(now)
	b := l.refill(key, now)
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// Blocked reports whether the key has no tokens left, without consuming one.
func (l *rateLimiter) Blocked(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.refill(key, l.now()).tokens < 1
}

// RetryAfter is the wait until the next token, rounded up to whole seconds.
func (l *rateLimiter) RetryAfter(key string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	b := l.refill(key, l.now())
	if b.tokens >= 1 {
		return 0
	}
	return int(math.Ceil((1 - b.tokens) / l.perSec))
}

type authLimits struct {
	ip           *rateLimiter
	loginFailure *rateLimiter
}

func newAuthLimits() *authLimits {
	return &authLimits{
		ip:           newRateLimiter(20, 3*time.Second),
		loginFailure: newRateLimiter(5, time.Minute),
	}
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func tooMany(w http.ResponseWriter, retryAfter int) {
	if retryAfter < 1 {
		retryAfter = 1
	}
	w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
	writeErr(w, http.StatusTooManyRequests, "too many requests, retry later")
}

func (s *Server) limitByIP(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := clientIP(r)
		if !s.limits.ip.Allow(key) {
			tooMany(w, s.limits.ip.RetryAfter(key))
			return
		}
		h(w, r)
	}
}

func loginKey(identity string) string {
	return strings.ToLower(strings.TrimSpace(identity))
}
