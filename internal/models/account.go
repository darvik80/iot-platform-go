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

// AccountPluginPolicy represents per-account plugin enable/disable settings.
type AccountPluginPolicy struct {
	AccountID    int64  `json:"account_id"`
	PluginID     string `json:"plugin_id"`
	Enabled      bool   `json:"enabled"`
	SettingsJSON string `json:"settings_json"`
	UpdatedAt    int64  `json:"updated_at"`
}
