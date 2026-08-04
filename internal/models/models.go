package models

type Role string

const (
	RoleAdmin    Role = "admin"
	RoleOperator Role = "operator"
	RoleViewer   Role = "viewer"
)

// User is defined in user.go
// Product is defined in product.go

type Device struct {
	ID              int64    `json:"id"`
	ProductPK       int64    `json:"product_pk"`
	ProductID       string   `json:"product_id"`
	DeviceID        string   `json:"device_id"`
	Name            string   `json:"name"`
	Description     string   `json:"description"`
	Tags            []string `json:"tags"`
	AccessTokenHash string   `json:"-"`
	TokenPrefix     string   `json:"token_prefix"`
	Status          string   `json:"status"`
	LastSeenAt      *int64   `json:"last_seen_at"`
	LastIP          string   `json:"last_ip"`
	FwVersion       string   `json:"fw_version"`
	TopicStyle      string   `json:"topic_style"`
	Enabled         bool     `json:"enabled"`
	CreatedAt       int64    `json:"created_at"`
	UpdatedAt       int64    `json:"updated_at"`
}

type DeviceCredentials struct {
	ProductID   string `json:"product_id"`
	DeviceID    string `json:"device_id"`
	AccessToken string `json:"access_token"`
	Username    string `json:"username"` // RabbitMQ MQTT user = {product}.{device}
	ClientID    string `json:"client_id"`
	BrokerHost  string `json:"broker_host"`
	BrokerPort  int    `json:"broker_port"`
	TopicStyle  string `json:"topic_style"`
	Snippet     string `json:"snippet"`
}

type TelemetrySample struct {
	Key       string   `json:"key"`
	ValueNum  *float64 `json:"value_num,omitempty"`
	ValueText *string  `json:"value_text,omitempty"`
	ValueBool *bool    `json:"value_bool,omitempty"`
	TS        int64    `json:"ts"`
}

type AttributeMap map[string]any

type RPCCall struct {
	ID            int64   `json:"id"`
	DevicePK      int64   `json:"device_pk"`
	RequestID     uint32  `json:"request_id"`
	Method        string  `json:"method"`
	ParamsJSON    string  `json:"params_json"`
	Status        string  `json:"status"`
	Code          *int    `json:"code"`
	Message       string  `json:"message"`
	DataJSON      *string `json:"data_json"`
	CorrelationID string  `json:"correlation_id"`
	CreatedAt     int64   `json:"created_at"`
	CompletedAt   *int64  `json:"completed_at"`
}

type Event struct {
	ID         int64   `json:"id"`
	DevicePK   int64   `json:"device_pk"`
	ProductID  string  `json:"product_id,omitempty"`
	DeviceID   string  `json:"device_id,omitempty"`
	EventID    string  `json:"event_id"`
	Type       string  `json:"type"`
	Severity   string  `json:"severity"`
	Code       string  `json:"code"`
	Message    string  `json:"message"`
	DataJSON   *string `json:"data_json"`
	TS         int64   `json:"ts"`
	IngestedAt int64   `json:"ingested_at"`
}

type DeviceLog struct {
	ID          int64   `json:"id"`
	DevicePK    int64   `json:"device_pk"`
	ProductID   string  `json:"product_id,omitempty"`
	DeviceID    string  `json:"device_id,omitempty"`
	Level       string  `json:"level"`
	Module      string  `json:"module"`
	Code        string  `json:"code"`
	Message     string  `json:"message"`
	ContextJSON *string `json:"context_json"`
	TS          int64   `json:"ts"`
	IngestedAt  int64   `json:"ingested_at"`
}

type OTAFirmware struct {
	ID         int64  `json:"id"`
	ProductPK  int64  `json:"product_pk"`
	ProductID  string `json:"product_id,omitempty"`
	Module     string `json:"module"`
	Version    string `json:"version"`
	Size       int64  `json:"size"`
	SHA256     string `json:"sha256"`
	SignMethod string `json:"sign_method"`
	Sign       string `json:"sign"`
	URL        string `json:"url"`
	CreatedAt  int64  `json:"created_at"`
}

type OTAJob struct {
	ID         int64  `json:"id"`
	DevicePK   int64  `json:"device_pk"`
	FirmwareID int64  `json:"firmware_id"`
	Status     string `json:"status"`
	Step       int    `json:"step"`
	Desc       string `json:"desc"`
	CreatedAt  int64  `json:"created_at"`
	UpdatedAt  int64  `json:"updated_at"`
}

type Overview struct {
	OnlineDevices  int   `json:"online_devices"`
	OfflineDevices int   `json:"offline_devices"`
	UnknownDevices int   `json:"unknown_devices"`
	TotalDevices   int   `json:"total_devices"`
	RecentEvents   int   `json:"recent_events_24h"`
	PlatformName   string `json:"platform_name"`
	MQTTConnected  bool  `json:"mqtt_connected"`
	NodeRedURL     string `json:"nodered_url"`
}
