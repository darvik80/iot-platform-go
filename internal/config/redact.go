package config

import (
	"net/url"
	"regexp"
	"strings"
)

// DevJWTSecret is the publicly known fallback; the platform never signs tokens with it.
const DevJWTSecret = "dev-jwt-secret-change-me"

var kvPasswordRe = regexp.MustCompile(`(?i)(password\s*=\s*)('[^']*'|\S+)`)

// RedactDSN hides the password in a database DSN so it can be logged or
// shown in the UI. Handles URL DSNs (postgres://user:pass@host/db) and
// key=value DSNs (host=... password=...). SQLite paths are returned as is.
func RedactDSN(dsn string) string {
	if dsn == "" {
		return dsn
	}
	if strings.Contains(dsn, "://") {
		if u, err := url.Parse(dsn); err == nil && u.User != nil {
			if _, has := u.User.Password(); has {
				u.User = url.UserPassword(u.User.Username(), "xxxxx")
			}
			q := u.Query()
			if q.Has("password") {
				q.Set("password", "xxxxx")
				u.RawQuery = q.Encode()
			}
			return u.String()
		}
	}
	return kvPasswordRe.ReplaceAllString(dsn, "${1}xxxxx")
}
