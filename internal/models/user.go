package models

type User struct {
	ID            int64  `json:"id"`
	Login         string `json:"login"`
	Email         string `json:"email"`
	DisplayName   string `json:"display_name"`
	PasswordHash  string `json:"-"`
	Role          Role   `json:"role"`
	EmailVerified bool   `json:"email_verified"`
	GoogleSub     string `json:"google_sub,omitempty"`
	Disabled      bool   `json:"disabled"`
	CreatedAt     int64  `json:"created_at"`
	UpdatedAt     int64  `json:"updated_at"`
	AuthProvider  string `json:"auth_provider,omitempty"` // password|google|both
}
