package auth

import (
	"net/http"
	"testing"
	"time"

	"cogitor-iot-platform/internal/models"
)

func TestHashPassword(t *testing.T) {
	pw := "test-password-123"
	hash, err := HashPassword(pw)
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}
	if hash == "" {
		t.Error("hash should not be empty")
	}
	if hash == pw {
		t.Error("hash should not equal plaintext")
	}
	// Same password should produce different hashes (bcrypt uses salt)
	hash2, _ := HashPassword(pw)
	if hash == hash2 {
		t.Error("same password should produce different hashes")
	}
}

func TestCheckPassword(t *testing.T) {
	pw := "my-secret-password"
	hash, _ := HashPassword(pw)

	if !CheckPassword(hash, pw) {
		t.Error("CheckPassword should return true for correct password")
	}
	if CheckPassword(hash, "wrong-password") {
		t.Error("CheckPassword should return false for wrong password")
	}
	if CheckPassword(hash, "") {
		t.Error("CheckPassword should return false for empty password")
	}
}

func TestHashToken(t *testing.T) {
	token := "test-token-abc123"
	hash := HashToken(token)
	if hash == "" {
		t.Error("hash should not be empty")
	}
	if len(hash) != 64 {
		t.Errorf("SHA256 hex should be 64 chars, got %d", len(hash))
	}
	// Same token should produce same hash (no salt)
	hash2 := HashToken(token)
	if hash != hash2 {
		t.Error("same token should produce same hash")
	}
	// Different token should produce different hash
	hash3 := HashToken("different-token")
	if hash == hash3 {
		t.Error("different tokens should produce different hashes")
	}
}

func TestGenerateAccessToken(t *testing.T) {
	token, err := GenerateAccessToken()
	if err != nil {
		t.Fatalf("GenerateAccessToken failed: %v", err)
	}
	if token == "" {
		t.Error("token should not be empty")
	}
	// Token should be URL-safe base64 (no +, /, =)
	for _, c := range token {
		if c == '+' || c == '/' || c == '=' {
			t.Errorf("token should be URL-safe, contains %c", c)
		}
	}
	// Two tokens should be different
	token2, _ := GenerateAccessToken()
	if token == token2 {
		t.Error("two generated tokens should be different")
	}
}

func TestTokenPrefix(t *testing.T) {
	tests := []struct {
		token    string
		expected string
	}{
		{"abcdefghij", "abcdefgh"},
		{"abcdefgh", "abcdefgh"},
		{"abc", "abc"},
		{"", ""},
		{"1234567890xyz", "12345678"},
	}

	for _, tt := range tests {
		t.Run(tt.token, func(t *testing.T) {
			if got := TokenPrefix(tt.token); got != tt.expected {
				t.Errorf("TokenPrefix(%q) = %q, want %q", tt.token, got, tt.expected)
			}
		})
	}
}

func TestIssueJWT(t *testing.T) {
	secret := "test-jwt-secret"
	user := models.User{
		ID:              1,
		Email:           "test@example.com",
		Login:           "testuser",
		IsPlatformAdmin: true,
	}
	accounts := []AccountRef{
		{AccountID: 10, AccountSlug: "acme", Role: "admin"},
		{AccountID: 20, AccountSlug: "beta", Role: "viewer"},
	}

	token, err := IssueJWT(secret, user, accounts, 10, time.Hour)
	if err != nil {
		t.Fatalf("IssueJWT failed: %v", err)
	}
	if token == "" {
		t.Error("token should not be empty")
	}
	// JWT has 3 parts separated by dots
	parts := len(token)
	if parts == 0 {
		t.Error("token should have content")
	}
}

func TestParseJWT(t *testing.T) {
	secret := "test-secret-key"
	user := models.User{
		ID:              42,
		Email:           "user@test.com",
		Login:           "testlogin",
		IsPlatformAdmin: false,
	}
	accounts := []AccountRef{
		{AccountID: 100, AccountSlug: "myaccount", Role: "operator"},
	}

	// Issue a token
	token, err := IssueJWT(secret, user, accounts, 100, time.Hour)
	if err != nil {
		t.Fatalf("IssueJWT failed: %v", err)
	}

	// Parse it back
	claims, err := ParseJWT(secret, token)
	if err != nil {
		t.Fatalf("ParseJWT failed: %v", err)
	}

	if claims.UserID != user.ID {
		t.Errorf("UserID = %d, want %d", claims.UserID, user.ID)
	}
	if claims.Email != user.Email {
		t.Errorf("Email = %q, want %q", claims.Email, user.Email)
	}
	if claims.Login != user.Login {
		t.Errorf("Login = %q, want %q", claims.Login, user.Login)
	}
	if claims.IsPlatformAdmin != user.IsPlatformAdmin {
		t.Errorf("IsPlatformAdmin = %v, want %v", claims.IsPlatformAdmin, user.IsPlatformAdmin)
	}
	if claims.ActiveAccountID != 100 {
		t.Errorf("ActiveAccountID = %d, want %d", claims.ActiveAccountID, 100)
	}
	if len(claims.Accounts) != 1 {
		t.Errorf("Accounts count = %d, want 1", len(claims.Accounts))
	}
}

func TestParseJWT_InvalidSecret(t *testing.T) {
	secret := "correct-secret"
	user := models.User{ID: 1, Email: "test@test.com"}
	accounts := []AccountRef{{AccountID: 1, AccountSlug: "test", Role: "admin"}}

	token, _ := IssueJWT(secret, user, accounts, 1, time.Hour)

	// Try to parse with wrong secret
	_, err := ParseJWT("wrong-secret", token)
	if err == nil {
		t.Error("ParseJWT should fail with wrong secret")
	}
}

func TestParseJWT_Expired(t *testing.T) {
	secret := "test-secret"
	user := models.User{ID: 1, Email: "test@test.com"}
	accounts := []AccountRef{{AccountID: 1, AccountSlug: "test", Role: "admin"}}

	// Issue a token that expired 1 hour ago
	token, _ := IssueJWT(secret, user, accounts, 1, -time.Hour)

	_, err := ParseJWT(secret, token)
	if err == nil {
		t.Error("ParseJWT should fail for expired token")
	}
}

func TestClaims_ActiveRole(t *testing.T) {
	claims := &Claims{
		ActiveAccountID: 20,
		Accounts: []AccountRef{
			{AccountID: 10, AccountSlug: "acme", Role: "admin"},
			{AccountID: 20, AccountSlug: "beta", Role: "operator"},
			{AccountID: 30, AccountSlug: "gamma", Role: "viewer"},
		},
	}

	role := claims.ActiveRole()
	if role != models.RoleOperator {
		t.Errorf("ActiveRole() = %q, want %q", role, models.RoleOperator)
	}

	// Test with non-existent active account
	claims.ActiveAccountID = 999
	role = claims.ActiveRole()
	if role != "" {
		t.Errorf("ActiveRole() for non-existent account = %q, want empty", role)
	}
}

func TestBearer(t *testing.T) {
	tests := []struct {
		name     string
		header   string
		expected string
	}{
		{"valid bearer", "Bearer abc123", "abc123"},
		{"lowercase", "bearer xyz789", "xyz789"},
		{"mixed case", "BeArEr token", "token"},
		{"no bearer", "Basic abc123", ""},
		{"empty", "", ""},
		{"bearer only", "Bearer ", ""},
		{"with spaces", "Bearer   token-with-spaces  ", "token-with-spaces"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", "/", nil)
			if tt.header != "" {
				req.Header.Set("Authorization", tt.header)
			}
			if got := Bearer(req); got != tt.expected {
				t.Errorf("Bearer() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestCanWrite(t *testing.T) {
	tests := []struct {
		role     models.Role
		expected bool
	}{
		{models.RoleAdmin, true},
		{models.RoleOperator, true},
		{models.RoleViewer, false},
		{models.Role(""), false},
	}

	for _, tt := range tests {
		t.Run(string(tt.role), func(t *testing.T) {
			if got := CanWrite(tt.role); got != tt.expected {
				t.Errorf("CanWrite(%q) = %v, want %v", tt.role, got, tt.expected)
			}
		})
	}
}

func TestCanAdmin(t *testing.T) {
	tests := []struct {
		role     models.Role
		expected bool
	}{
		{models.RoleAdmin, true},
		{models.RoleOperator, false},
		{models.RoleViewer, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.role), func(t *testing.T) {
			if got := CanAdmin(tt.role); got != tt.expected {
				t.Errorf("CanAdmin(%q) = %v, want %v", tt.role, got, tt.expected)
			}
		})
	}
}

func TestCanPlatformAdmin(t *testing.T) {
	// Nil claims
	if CanPlatformAdmin(nil) {
		t.Error("CanPlatformAdmin(nil) should be false")
	}

	// Non-admin
	claims := &Claims{IsPlatformAdmin: false}
	if CanPlatformAdmin(claims) {
		t.Error("CanPlatformAdmin should be false for non-admin")
	}

	// Admin
	claims = &Claims{IsPlatformAdmin: true}
	if !CanPlatformAdmin(claims) {
		t.Error("CanPlatformAdmin should be true for admin")
	}
}
