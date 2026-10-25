package models

type MetricSource struct {
	DeviceID int64  `json:"device_id"`
	Key      string `json:"key"`
	Label    string `json:"label"`
	Color    string `json:"color"`
	Scope    *Scope `json:"scope,omitempty"`
}

type MonitoringWidget struct {
	ID            string         `json:"id"`
	Title         string         `json:"title"`
	Type          string         `json:"type"`
	Sources       []MetricSource `json:"sources"`
	WindowSeconds int            `json:"window_seconds"`
	Aggregation   string         `json:"aggregation"`
	Width         int            `json:"width"`
	Min           float64        `json:"min"`
	Max           float64        `json:"max"`
}

type MonitoringDashboard struct {
	ID        int64              `json:"id"`
	AccountID int64              `json:"account_id"`
	Name      string             `json:"name"`
	Version   int64              `json:"version"`
	Widgets   []MonitoringWidget `json:"widgets"`
}

type MonitoringMetric struct {
	Key      string `json:"key"`
	Kind     string `json:"kind"`
	LastSeen int64  `json:"last_seen"`
}

type MonitoringQuery struct {
	Widget MonitoringWidget `json:"widget"`
	From   int64            `json:"from"`
	To     int64            `json:"to"`
}

type MonitoringPoint struct {
	TS    int64 `json:"ts"`
	Value any   `json:"value"`
}

type MonitoringSeries struct {
	DeviceID int64             `json:"device_id"`
	Key      string            `json:"key"`
	Label    string            `json:"label"`
	Color    string            `json:"color"`
	Points   []MonitoringPoint `json:"points"`
}

type MonitoringData struct {
	From   int64              `json:"from"`
	To     int64              `json:"to"`
	Series []MonitoringSeries `json:"series"`
}

type AlertRule struct {
	ID              int64   `json:"id"`
	AccountID       int64   `json:"account_id"`
	Name            string  `json:"name"`
	DeviceID        int64   `json:"device_id"`
	Metric          string  `json:"metric"`
	Kind            string  `json:"kind"`
	Operator        string  `json:"operator"`
	Threshold       float64 `json:"threshold"`
	Recovery        float64 `json:"recovery"`
	ForSeconds      int     `json:"for_seconds"`
	MissingSeconds  int     `json:"missing_seconds"`
	CooldownSeconds int     `json:"cooldown_seconds"`
	Enabled         bool    `json:"enabled"`
	ChannelIDs      []int64 `json:"channel_ids"`
	State           string  `json:"state"`
	Since           int64   `json:"since"`
	Scope           *Scope  `json:"scope,omitempty"`
}

type NotificationChannel struct {
	ID          int64  `json:"id"`
	AccountID   int64  `json:"account_id"`
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	Destination string `json:"destination"`
	Enabled     bool   `json:"enabled"`
	HasSecret   bool   `json:"has_secret"`
}

type NotificationChannelInput struct {
	NotificationChannel
	Secret string `json:"secret"`
}

type AlertEvent struct {
	ID        int64  `json:"id"`
	RuleID    int64  `json:"rule_id"`
	Name      string `json:"name"`
	State     string `json:"state"`
	Message   string `json:"message"`
	CreatedAt int64  `json:"created_at"`
}

type NotificationDelivery struct {
	ID          int64  `json:"id"`
	EventID     int64  `json:"event_id"`
	ChannelID   int64  `json:"channel_id"`
	Status      string `json:"status"`
	Attempts    int    `json:"attempts"`
	LastError   string `json:"last_error"`
	NextAttempt int64  `json:"next_attempt"`
}
