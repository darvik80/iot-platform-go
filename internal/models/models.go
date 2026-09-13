package models

import "encoding/json"

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
	AutoOTA         bool     `json:"auto_ota"`
	Enabled         bool     `json:"enabled"`
	CreatedAt       int64    `json:"created_at"`
	UpdatedAt       int64    `json:"updated_at"`
}

type DeviceProvisionWiFi struct {
	SSID     string `json:"ssid"`
	Password string `json:"password"`
}

type DeviceProvisionCogitor struct {
	Product    string `json:"product"`
	Device     string `json:"device"`
	Host       string `json:"host"`
	Port       int    `json:"port"`
	WSURL      string `json:"ws_url,omitempty"`
	WSPort     int    `json:"ws_port,omitempty"`
	Token      string `json:"token"`
	TLS        bool   `json:"tls"`
	TopicShort bool   `json:"topic_short"`
}

type DeviceProvision struct {
	WiFi    DeviceProvisionWiFi    `json:"wifi"`
	Cogitor DeviceProvisionCogitor `json:"cogitor"`
}

type DeviceCredentials struct {
	ProductID    string           `json:"product_id"`
	DeviceID     string           `json:"device_id"`
	AccessToken  string           `json:"access_token"`
	Username     string           `json:"username"` // MQTT user = {product}.{device}
	ClientID     string           `json:"client_id"`
	BrokerHost   string           `json:"broker_host"`
	BrokerPort   int              `json:"broker_port"`
	BrokerWSURL  string           `json:"broker_ws_url,omitempty"`
	BrokerWSPort int              `json:"broker_ws_port,omitempty"`
	TopicStyle   string           `json:"topic_style"`
	Snippet      string           `json:"snippet"`
	Provision    *DeviceProvision `json:"provision,omitempty"`
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
	ID            int64  `json:"id"`
	ProductPK     int64  `json:"product_pk"`
	ProductID     string `json:"product_id,omitempty"`
	Module        string `json:"module"`
	Version       string `json:"version"`
	Size          int64  `json:"size"`
	SHA256        string `json:"sha256"`
	SignMethod    string `json:"sign_method"`
	Sign          string `json:"sign"`
	URL           string `json:"url"`
	DownloadToken string `json:"download_token,omitempty"`
	Hosted        bool   `json:"hosted"`
	CreatedAt     int64  `json:"created_at"`
}

type OTAJob struct {
	ID              int64  `json:"id"`
	DevicePK        int64  `json:"device_pk"`
	ProductID       string `json:"product_id,omitempty"`
	DeviceID        string `json:"device_id,omitempty"`
	FirmwareID      int64  `json:"firmware_id"`
	FirmwareVersion string `json:"firmware_version,omitempty"`
	Module          string `json:"module,omitempty"`
	Status          string `json:"status"`
	Step            int    `json:"step"`
	Desc            string `json:"desc"`
	CreatedAt       int64  `json:"created_at"`
	UpdatedAt       int64  `json:"updated_at"`
}

type OTANotifyResult struct {
	FirmwareID int64    `json:"firmware_id"`
	Started    int      `json:"started"`
	Skipped    int      `json:"skipped"`
	Failed     int      `json:"failed"`
	Jobs       []OTAJob `json:"jobs"`
	Errors     []string `json:"errors,omitempty"`
}

type Overview struct {
	OnlineDevices  int    `json:"online_devices"`
	OfflineDevices int    `json:"offline_devices"`
	UnknownDevices int    `json:"unknown_devices"`
	TotalDevices   int    `json:"total_devices"`
	RecentEvents   int    `json:"recent_events_24h"`
	PlatformName   string `json:"platform_name"`
	MQTTConnected  bool   `json:"mqtt_connected"`
	MQTTClients    int    `json:"mqtt_clients"`
	NodeRedURL     string `json:"nodered_url"`
}

type MQTTClient struct {
	ClientID      string   `json:"client_id"`
	Username      string   `json:"username"`
	Remote        string   `json:"remote"`
	Listener      string   `json:"listener,omitempty"`
	Kind          string   `json:"kind"`
	DevicePK      int64    `json:"device_pk,omitempty"`
	Protocol      byte     `json:"protocol,omitempty"`
	Clean         bool     `json:"clean"`
	Subscriptions []string `json:"subscriptions"`
	ConnectedAt   int64    `json:"connected_at,omitempty"`
	LastSeenAt    int64    `json:"last_seen_at,omitempty"`
}

type MQTTEvent struct {
	TS       int64  `json:"ts"`
	Level    string `json:"level"`
	Kind     string `json:"kind"`
	ClientID string `json:"client_id,omitempty"`
	Username string `json:"username,omitempty"`
	Remote   string `json:"remote,omitempty"`
	Topic    string `json:"topic,omitempty"`
	Message  string `json:"message,omitempty"`
	Bytes    int    `json:"bytes,omitempty"`
}

type MQTTStatus struct {
	Listening   bool         `json:"listening"`
	Addr        string       `json:"addr,omitempty"`
	WSAddr      string       `json:"ws_addr,omitempty"`
	TLSAddr     string       `json:"tls_addr,omitempty"`
	ClientCount int          `json:"client_count"`
	Clients     []MQTTClient `json:"clients"`
	Events      []MQTTEvent  `json:"events"`
}

type PluginInfo struct {
	ID            string          `json:"id"`
	Name          string          `json:"name"`
	Description   string          `json:"description"`
	DefaultConfig json.RawMessage `json:"default_config"`
	Effects       []string        `json:"effects,omitempty"`
	Form          json.RawMessage `json:"form,omitempty"`
}

type DevicePluginBinding struct {
	ID          int64           `json:"id"`
	DevicePK    int64           `json:"device_pk"`
	PluginID    string          `json:"plugin_id"`
	Enabled     bool            `json:"enabled"`
	Config      json.RawMessage `json:"config"`
	CreatedAt   int64           `json:"created_at"`
	UpdatedAt   int64           `json:"updated_at"`
	Name        string          `json:"name,omitempty"`
	Description string          `json:"description,omitempty"`
	Effects     []string        `json:"effects,omitempty"`
	Form        json.RawMessage `json:"form,omitempty"`
	Running     bool            `json:"running,omitempty"`
	LastError   string          `json:"last_error,omitempty"`
}

// ExternalPlugin is an out-of-process plugin registered via API.
type ExternalPlugin struct {
	ID              string          `json:"id"`
	AccountID       int64           `json:"account_id"`
	Name            string          `json:"name"`
	Description     string          `json:"description"`
	Endpoint        string          `json:"endpoint"`
	GRPCEndpoint    string          `json:"grpc_endpoint,omitempty"`
	AuthTokenHash   string          `json:"-"`
	Auto            bool            `json:"auto"`
	IntervalMS      int             `json:"interval_ms"`
	DefaultConfig   json.RawMessage `json:"default_config"`
	Effects         []string        `json:"effects,omitempty"`
	Form            json.RawMessage `json:"form,omitempty"`
	Mode            string          `json:"mode"`   // webhook | polling | grpc
	Status          string          `json:"status"` // online | offline | unknown
	LastHeartbeatAt *int64          `json:"last_heartbeat_at,omitempty"`
	MQTTUser        string          `json:"mqtt_user,omitempty"`
	MQTTPassHash    string          `json:"-"`
	CreatedAt       int64           `json:"created_at"`
	UpdatedAt       int64           `json:"updated_at"`
}

// ExternalPluginEvent is a queued event for polling-mode plugins.
type ExternalPluginEvent struct {
	ID        int64           `json:"id"`
	PluginID  string          `json:"plugin_id"`
	BindingID int64           `json:"binding_id"`
	Method    string          `json:"method"` // plugin.apply_config | plugin.on_event | plugin.detach
	Params    json.RawMessage `json:"params"`
	CreatedAt int64           `json:"created_at"`
	AckedAt   *int64          `json:"acked_at,omitempty"`
}
