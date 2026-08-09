// Package plugins is in-process per-device automation (not Node-RED).
//
// Catalog: Register(Descriptor, Factory) in init(). One Go file per plugin.
// Binding: SQLite device_plugin_bindings, UNIQUE(device_pk, plugin_id).
// Same catalog plugin may be bound to many devices (independent instances).
//
// Hub (services.PluginHub) lifecycle per binding:
//
//	ApplyConfig(json) → runner
//	device online + Auto() → ticker → OnEvent(tick)
//	save / Apply now → OnEvent(apply)
//	MQTT status/telemetry/event/rpc/result → OnEvent(kind)
//	offline → ticker stops; next online is a fresh attach/apply opportunity
//
// Plugins must not talk MQTT or tokens. Only Runtime.InvokeRPC + Event.
package plugins

import (
	"encoding/json"
	"sync"
)

const (
	EventTick      = "tick"
	EventApply     = "apply"
	EventStatus    = "status"
	EventTelemetry = "telemetry"
	EventEvent     = "event"
	EventRPCResult = "rpc/result"
)

type Event struct {
	Kind     string // tick | apply | status | telemetry | event | rpc/result
	Online   bool
	Payload  any
	DevicePK int64
}

type Runtime interface {
	InvokeRPC(method string, params map[string]any) error
}

type Instance interface {
	ApplyConfig(raw json.RawMessage) error
	OnEvent(rt Runtime, ev Event) error
	IntervalMS() int
	Auto() bool
}

type Descriptor struct {
	ID            string          `json:"id"`
	Name          string          `json:"name"`
	Description   string          `json:"description"`
	DefaultConfig json.RawMessage `json:"default_config"`
	Effects       []string        `json:"effects,omitempty"`
}

type Factory func() Instance

var (
	regMu     sync.RWMutex
	factories = map[string]Factory{}
	meta      = map[string]Descriptor{}
)

func Register(d Descriptor, f Factory) {
	regMu.Lock()
	defer regMu.Unlock()
	factories[d.ID] = f
	meta[d.ID] = d
}

func Lookup(id string) (Descriptor, Factory, bool) {
	regMu.RLock()
	defer regMu.RUnlock()
	d, ok := meta[id]
	if !ok {
		return Descriptor{}, nil, false
	}
	return d, factories[id], true
}

func Catalog() []Descriptor {
	regMu.RLock()
	defer regMu.RUnlock()
	out := make([]Descriptor, 0, len(meta))
	for _, d := range meta {
		out = append(out, d)
	}
	return out
}
