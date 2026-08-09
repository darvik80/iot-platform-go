package plugins

import "encoding/json"

// FormField is the plugin-owned config UI. Web dashboard only renders this schema.
type FormField struct {
	Key          string          `json:"key"`
	Type         string          `json:"type"` // number|string|select|checkbox|time|timezone|json|weekdays|object|array|break
	Label        string          `json:"label,omitempty"`
	Help         string          `json:"help,omitempty"`
	Min          *float64        `json:"min,omitempty"`
	Max          *float64        `json:"max,omitempty"`
	Options      []FormOpt       `json:"options,omitempty"`
	OptionsFrom  string          `json:"options_from,omitempty"` // "effects" → Descriptor.Effects
	DisabledWhen map[string]any  `json:"disabled_when,omitempty"`
	Fields       []FormField     `json:"fields,omitempty"`
	Add          json.RawMessage `json:"add,omitempty"`
	AddLabel     string          `json:"add_label,omitempty"`
	RemoveLabel  string          `json:"remove_label,omitempty"`
	Placeholder  string          `json:"placeholder,omitempty"`
	Suggest      []int           `json:"suggest,omitempty"`
	Wide         bool            `json:"wide,omitempty"`
}

type FormOpt struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

func FormJSON(fields []FormField) json.RawMessage {
	if len(fields) == 0 {
		return nil
	}
	b, err := json.Marshal(fields)
	if err != nil {
		return nil
	}
	return b
}

func num(v float64) *float64 { return &v }
