package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"cogitor-iot-platform/internal/models"
)

// AccountRef is a compact per-account role descriptor embedded in JWT claims.
type AccountRef struct {
	AccountID   int64  `json:"account_id"`
	AccountSlug string `json:"account_slug"`
	Role        string `json:"role"`
}

type Claims struct {
	UserID          int64        `json:"uid"`
	Email           string       `json:"email"`
	Login           string       `json:"login"`
	Accounts        []AccountRef `json:"accounts"`
	ActiveAccountID int64        `json:"active_account_id"`
	jwt.RegisteredClaims
}

// ActiveRole returns the role for the currently active account.
func (c *Claims) ActiveRole() models.Role {
	for _, a := range c.Accounts {
		if a.AccountID == c.ActiveAccountID {
			return models.Role(a.Role)
		}
	}
	return ""
}

func HashPassword(pw string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	return string(b), err
}

func CheckPassword(hash, pw string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw)) == nil
}

func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func GenerateAccessToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func TokenPrefix(token string) string {
	if len(token) <= 8 {
		return token
	}
	return token[:8]
}

// IssueJWT creates a signed JWT carrying multi-account claims.
// accounts must be non-empty; activeAccountID must be one of the account IDs.
func IssueJWT(secret string, u models.User, accounts []AccountRef, activeAccountID int64, ttl time.Duration) (string, error) {
	claims := Claims{
		UserID:          u.ID,
		Email:           u.Email,
		Login:           u.Login,
		Accounts:        accounts,
		ActiveAccountID: activeAccountID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(ttl)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Subject:   fmt.Sprintf("%d", u.ID),
		},
	}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return t.SignedString([]byte(secret))
}

func ParseJWT(secret, token string) (*Claims, error) {
	parsed, err := jwt.ParseWithClaims(token, &Claims{}, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(secret), nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := parsed.Claims.(*Claims)
	if !ok || !parsed.Valid {
		return nil, errors.New("invalid token")
	}
	return claims, nil
}

func Bearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if strings.HasPrefix(strings.ToLower(h), "bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}

// CanWrite returns true if the per-account role allows write operations.
func CanWrite(role models.Role) bool {
	return role == models.RoleAdmin || role == models.RoleOperator
}

// CanAdmin returns true if the per-account role is admin.
func CanAdmin(role models.Role) bool {
	return role == models.RoleAdmin
}
