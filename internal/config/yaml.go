package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// LoadYAML reads a YAML config file and sets values as env vars (does not override existing).
// Nested keys are joined with "_", e.g. mqtt.addr → MQTT_ADDR, server.public_base_url → SERVER_PUBLIC_BASE_URL.
func LoadYAML(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return
	}
	flat := make(map[string]string)
	flatten("", raw, flat)
	for k, v := range flat {
		if _, exists := os.LookupEnv(k); !exists {
			_ = os.Setenv(k, v)
		}
	}
}

// flatten recursively walks nested maps and produces SCREAMING_SNAKE_CASE keys.
func flatten(prefix string, m map[string]any, out map[string]string) {
	for k, v := range m {
		key := strings.ToUpper(k)
		if prefix != "" {
			key = prefix + "_" + key
		}
		switch val := v.(type) {
		case map[string]any:
			flatten(key, val, out)
		default:
			out[key] = fmt.Sprintf("%v", val)
		}
	}
}
