package models

type User struct {
	ID              int64  `json:"id"`
	Login           string `json:"login"`
	Email           string `json:"email"`
	DisplayName     string `json:"display_name"`
	PasswordHash    string `json:"-"`
	Role            Role   `json:"role"`
	EmailVerified   bool   `json:"email_verified"`
	GoogleSub       string `json:"google_sub,omitempty"`
	Disabled        bool   `json:"disabled"`
	IsPlatformAdmin bool   `json:"is_platform_admin"`
	CreatedAt       int64  `json:"created_at"`
	UpdatedAt       int64  `json:"updated_at"`
	AuthProvider    string `json:"auth_provider,omitempty"` // password|google|both
	AvatarURL       string `json:"avatar_url"`
}

// APIToken is a long-lived credential for MCP / scripts (plaintext shown once at create).
type APIToken struct {
	ID         int64  `json:"id"`
	UserID     int64  `json:"user_id"`
	AccountID  *int64 `json:"account_id,omitempty"`
	Name       string `json:"name"`
	Token      string `json:"token,omitempty"` // plaintext — only on create
	Prefix     string `json:"token_prefix"`
	CreatedAt  int64  `json:"created_at"`
	LastUsedAt *int64 `json:"last_used_at,omitempty"`
}
