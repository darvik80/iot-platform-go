package models

type ScopeType string

const (
	ScopeDevice  ScopeType = "device"
	ScopeGroup   ScopeType = "group"
	ScopeProduct ScopeType = "product"
	ScopeTag     ScopeType = "tag"
)

type Scope struct {
	Type      ScopeType `json:"type"`
	DeviceID  int64     `json:"device_id,omitempty"`
	GroupID   int64     `json:"group_id,omitempty"`
	ProductID int64     `json:"product_id,omitempty"`
	Tag       string    `json:"tag,omitempty"`
}

type DeviceGroup struct {
	ID          int64  `json:"id"`
	AccountID   int64  `json:"account_id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	ScopeType   string `json:"scope_type"`
	ScopeFilter string `json:"scope_filter"`
	CreatedAt   int64  `json:"created_at"`
	UpdatedAt   int64  `json:"updated_at"`
}

type DeviceGroupInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	ScopeType   string `json:"scope_type"`
	ScopeFilter string `json:"scope_filter"`
}

type DeviceGroupMembers struct {
	DeviceIDs []int64 `json:"device_ids"`
}
