package models

// ProductMethod describes an RPC method available for devices of a product.
type ProductMethod struct {
	Name        string               `json:"name"`
	Label       string               `json:"label,omitempty"`
	Description string               `json:"description,omitempty"`
	Params      []ProductMethodParam `json:"params,omitempty"`
}

type ProductMethodParam struct {
	Name        string `json:"name"`
	Label       string `json:"label,omitempty"`
	Type        string `json:"type"` // number|string|boolean|json
	Default     any    `json:"default,omitempty"`
	Required    bool   `json:"required,omitempty"`
	Description string `json:"description,omitempty"`
}

// ProductMetric describes how to display a telemetry key.
type ProductMetric struct {
	Key         string `json:"key"`
	Label       string `json:"label,omitempty"`
	Description string `json:"description,omitempty"`
	Group       string `json:"group,omitempty"`  // System|Network|Memory|Storage|Sensors|Other
	Unit        string `json:"unit,omitempty"`   // %, °C, dBm, bytes, seconds, …
	Format      string `json:"format,omitempty"` // bytes|percent|duration|bool|number|text|rssi
	Precision   *int   `json:"precision,omitempty"`
}

// ProductAttribute describes reported/desired property display + editing.
type ProductAttribute struct {
	Key         string `json:"key"`
	Label       string `json:"label,omitempty"`
	Description string `json:"description,omitempty"`
	Scope       string `json:"scope"` // reported|desired|both
	Type        string `json:"type"`  // number|string|boolean|json
	Unit        string `json:"unit,omitempty"`
	Group       string `json:"group,omitempty"`
	Writable    bool   `json:"writable,omitempty"` // desired editor
}

type Product struct {
	ID                int64              `json:"id"`
	AccountID         int64              `json:"account_id"`
	ProductID         string             `json:"product_id"`
	Name              string             `json:"name"`
	Description       string             `json:"description"`
	TopicStyleDefault string             `json:"topic_style_default"`
	AutoOTA           bool               `json:"auto_ota"`
	Methods           []ProductMethod    `json:"methods"`
	Metrics           []ProductMetric    `json:"metrics"`
	Attributes        []ProductAttribute `json:"attributes"`
	CreatedAt         int64              `json:"created_at"`
	UpdatedAt         int64              `json:"updated_at"`
}
