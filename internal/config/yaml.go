package config

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"

	"go.yaml.in/yaml/v4"
)

// LoadYAML reads a YAML config file and sets values as env vars (does not override existing).
// Nested keys are joined with "_", e.g. mqtt.addr → MQTT_ADDR, server.public_base_url → SERVER_PUBLIC_BASE_URL.
// Returns true if the file was found and parsed successfully.
func LoadYAML(path string) bool {
	ok, _ := LoadYAMLFile(path, false)
	return ok
}

// LoadYAMLForce reads a YAML config file and sets values as env vars, overriding existing values.
// This is used after setup wizard to ensure the new config is applied.
// Returns true if the file was found and parsed successfully.
func LoadYAMLForce(path string) bool {
	ok, _ := LoadYAMLFile(path, true)
	return ok
}

// Files lists the config files the platform reads, highest priority first.
// Both .yml and .yaml are accepted.
var Files = []string{
	"./config.yml", "./config.yaml",
	"/opt/iot-platform/etc/config.yml", "/opt/iot-platform/etc/config.yaml",
}

// LoadFiles loads every existing file from Files and returns the paths that
// were applied plus the reasons existing files were not.
func LoadFiles(force bool) (loaded []string, errs []error) {
	for _, p := range Files {
		ok, err := LoadYAMLFile(p, force)
		if ok {
			loaded = append(loaded, p)
		}
		if err != nil {
			errs = append(errs, err)
		}
	}
	return loaded, errs
}

// LoadYAMLFile is LoadYAML / LoadYAMLForce that also reports why an existing
// file was not applied (unreadable or invalid YAML). A missing file is not an error.
func LoadYAMLFile(path string, force bool) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	data = detabYAML(data)
	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return false, fmt.Errorf("parse %s: %w", path, err)
	}
	flat := make(map[string]string)
	flatten("", raw, flat)
	for k, v := range flat {
		if force {
			_ = os.Setenv(k, v)
		} else if _, exists := os.LookupEnv(k); !exists {
			_ = os.Setenv(k, v)
		}
	}
	return true, nil
}

// detabYAML replaces tab indentation with two spaces. YAML forbids tabs as
// indentation; a file that uses them fails to parse and the platform then
// falls back to compiled-in defaults.
func detabYAML(data []byte) []byte {
	if !bytes.Contains(data, []byte{'\t'}) {
		return data
	}
	lines := bytes.Split(data, []byte{'\n'})
	for i, line := range lines {
		n := 0
		for n < len(line) && (line[n] == ' ' || line[n] == '\t') {
			n++
		}
		if n == 0 {
			continue
		}
		width := 0
		for _, c := range line[:n] {
			if c == '\t' {
				width += 2
			} else {
				width++
			}
		}
		lines[i] = append(bytes.Repeat([]byte{' '}, width), line[n:]...)
	}
	return bytes.Join(lines, []byte{'\n'})
}

func asStringMap(v any) (map[string]any, bool) {
	switch m := v.(type) {
	case map[string]any:
		return m, true
	case map[any]any:
		out := make(map[string]any, len(m))
		for k, val := range m {
			out[fmt.Sprint(k)] = val
		}
		return out, true
	default:
		return nil, false
	}
}

// flatten recursively walks nested maps and produces SCREAMING_SNAKE_CASE keys.
func flatten(prefix string, m map[string]any, out map[string]string) {
	for k, v := range m {
		key := strings.ToUpper(k)
		if prefix != "" {
			key = prefix + "_" + key
		}
		if nested, ok := asStringMap(v); ok {
			flatten(key, nested, out)
			continue
		}
		out[key] = fmt.Sprintf("%v", v)
	}
}
