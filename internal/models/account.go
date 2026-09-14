package models

// Account represents a tenant in the multi-account system.
type Account struct {
	ID           int64  `json:"id"`
	Slug         string `json:"slug"`
	Name         string `json:"name"`
	SettingsJSON string `json:"settings_json"`
	Status       string `json:"status"` // active | suspended | deleted
	CreatedAt    int64  `json:"created_at"`
	UpdatedAt    int64  `json:"updated_at"`
}

// AccountMembership represents a user's membership in an account with a specific role.
type AccountMembership struct {
	UserID    int64  `json:"user_id"`
	AccountID int64  `json:"account_id"`
	Role      Role   `json:"role"` // admin | operator | viewer
	Status    string `json:"status"` // active | invited | suspended
	JoinedAt  int64  `json:"joined_at"`
	InvitedBy *int64 `json:"invited_by,omitempty"`
}

// AccountMembershipWithAccount enriches a membership with the account's slug, name and status.
type AccountMembershipWithAccount struct {
	AccountMembership
	AccountSlug   string `json:"account_slug"`
	AccountName   string `json:"account_name"`
	AccountStatus string `json:"account_status"` // active | suspended | deleted
}

// MemberInfo is a user's info together with their membership role/status in an account.
type MemberInfo struct {
	UserID      int64  `json:"user_id"`
	Login       string `json:"login"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	Role        Role   `json:"role"`
	Status      string `json:"status"`
	JoinedAt    int64  `json:"joined_at"`
}

// AccountPluginPolicy represents per-account plugin enable/disable settings.
type AccountPluginPolicy struct {
	AccountID    int64  `json:"account_id"`
	PluginID     string `json:"plugin_id"`
	Enabled      bool   `json:"enabled"`
	SettingsJSON string `json:"settings_json"`
	UpdatedAt    int64  `json:"updated_at"`
}

// AccountInvitation represents a pending invitation to join an account.
type AccountInvitation struct {
	ID             int64  `json:"id"`
	AccountID      int64  `json:"account_id"`
	Email          string `json:"email"`
	Role           Role   `json:"role"`
	TokenHash      string `json:"-"`
	Status         string `json:"status"` // pending | accepted | revoked | expired
	DeliveryMethod string `json:"delivery_method"` // email | link
	InvitedBy      int64  `json:"invited_by"`
	ExpiresAt      int64  `json:"expires_at"`
	CreatedAt      int64  `json:"created_at"`
	UpdatedAt      int64  `json:"updated_at"`
}

// InvitationInfo is the public view of an invitation (no secrets).
type InvitationInfo struct {
	AccountName string `json:"account_name"`
	AccountSlug string `json:"account_slug"`
	Email       string `json:"email"`
	Role        Role   `json:"role"`
	Status      string `json:"status"`
	ExpiresAt   int64  `json:"expires_at"`
}
