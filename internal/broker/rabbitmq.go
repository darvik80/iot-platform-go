package broker

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Manager provisions RabbitMQ MQTT users via the HTTP management API.
type Manager struct {
	baseURL  string
	username string
	password string
	client   *http.Client
}

func New(mgmtURL, username, password string) *Manager {
	return &Manager{
		baseURL:  strings.TrimRight(mgmtURL, "/"),
		username: username,
		password: password,
		client:   &http.Client{Timeout: 10 * time.Second},
	}
}

// Ping checks management API reachability with configured credentials.
func (m *Manager) Ping() error {
	req, err := http.NewRequest(http.MethodGet, m.baseURL+"/api/whoami", nil)
	if err != nil {
		return err
	}
	req.SetBasicAuth(m.username, m.password)
	resp, err := m.client.Do(req)
	if err != nil {
		return fmt.Errorf("rabbitmq management unreachable at %s: %w", m.baseURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("rabbitmq management auth failed: %s %s", resp.Status, string(b))
	}
	return nil
}

// UpsertDeviceUser creates/updates MQTT user username={product}.{device}, password=token.
func (m *Manager) UpsertDeviceUser(token, productID, deviceID string) error {
	if token == "" {
		return fmt.Errorf("empty access token")
	}
	user := DeviceMQTTUsername(productID, deviceID)
	if user == "." || productID == "" || deviceID == "" {
		return fmt.Errorf("empty product/device id")
	}
	body := map[string]any{
		"password": token,
		"tags":     "",
	}
	if err := m.put("/api/users/"+encodeUser(user), body); err != nil {
		return fmt.Errorf("create user: %w", err)
	}

	// MQTT needs rights to declare session queues and publish/consume via amq.topic.
	perms := map[string]any{
		"configure": mqttConfigureACL(),
		"write":     mqttWriteACL(productID, deviceID),
		"read":      mqttReadACL(productID, deviceID),
	}
	if err := m.put("/api/permissions/"+encodeVHost("/")+"/"+encodeUser(user), perms); err != nil {
		_ = m.DeleteUser(user)
		return fmt.Errorf("set permissions: %w", err)
	}

	topic := map[string]any{
		"exchange": "amq.topic",
		"write":    topicWriteACL(productID, deviceID),
		"read":     topicReadACL(productID, deviceID),
	}
	if err := m.put("/api/topic-permissions/"+encodeVHost("/")+"/"+encodeUser(user), topic); err != nil {
		_ = m.DeleteUser(user)
		return fmt.Errorf("set topic permissions: %w", err)
	}

	ok, err := m.UserExists(user)
	if err != nil {
		return fmt.Errorf("verify user: %w", err)
	}
	if !ok {
		return fmt.Errorf("user was not created (verify failed)")
	}
	return nil
}

// DeviceMQTTUsername is the RabbitMQ MQTT login for a device (not the secret token).
func DeviceMQTTUsername(productID, deviceID string) string {
	return productID + "." + deviceID
}

func (m *Manager) UserExists(name string) (bool, error) {
	req, err := http.NewRequest(http.MethodGet, m.baseURL+"/api/users/"+encodeUser(name), nil)
	if err != nil {
		return false, err
	}
	req.SetBasicAuth(m.username, m.password)
	resp, err := m.client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return false, fmt.Errorf("%s %s", resp.Status, string(b))
	}
	return true, nil
}

func (m *Manager) DeleteUser(name string) error {
	req, err := http.NewRequest(http.MethodDelete, m.baseURL+"/api/users/"+encodeUser(name), nil)
	if err != nil {
		return err
	}
	req.SetBasicAuth(m.username, m.password)
	resp, err := m.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode < 300 {
		return nil
	}
	b, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("delete rabbitmq user: %s %s", resp.Status, string(b))
}

func (m *Manager) EnsureNodeRedUser(user, pass string) error {
	if pass == "" {
		return fmt.Errorf("empty nodered password")
	}
	body := map[string]any{"password": pass, "tags": ""}
	if err := m.put("/api/users/"+encodeUser(user), body); err != nil {
		return err
	}
	perms := map[string]any{
		"configure": "^mqtt-subscription-.*$",
		"write":     "^(amq\\.topic|mqtt-subscription-.*|platform\\.v1\\.commands\\..*)$",
		"read":      "^(amq\\.topic|mqtt-subscription-.*|platform\\.v1\\.events\\..*)$",
	}
	if err := m.put("/api/permissions/"+encodeVHost("/")+"/"+encodeUser(user), perms); err != nil {
		return err
	}
	topic := map[string]any{
		"exchange": "amq.topic",
		"write":    "^platform\\.v1\\.commands\\.",
		"read":     "^platform\\.v1\\.events\\.",
	}
	return m.put("/api/topic-permissions/"+encodeVHost("/")+"/"+encodeUser(user), topic)
}

func (m *Manager) put(path string, body any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPut, m.baseURL+path, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(m.username, m.password)
	resp, err := m.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		rb, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%s %s", resp.Status, string(rb))
	}
	return nil
}

func encodeUser(name string) string {
	// PathEscape is too aggressive for RabbitMQ (encodes correctly, but double-check).
	return url.PathEscape(name)
}

func encodeVHost(vhost string) string {
	return url.PathEscape(vhost)
}

func mqttConfigureACL() string {
	// MQTT plugin creates queues named mqtt-subscription-<client_id|username>qos[0|1]
	return "^mqtt-subscription-.*$"
}

func mqttWriteACL(productID, deviceID string) string {
	_ = productID
	_ = deviceID
	return "^(amq\\.topic|mqtt-subscription-.*)$"
}

func mqttReadACL(productID, deviceID string) string {
	_ = productID
	_ = deviceID
	return "^(amq\\.topic|mqtt-subscription-.*)$"
}

func topicWriteACL(productID, deviceID string) string {
	fullUp := fmt.Sprintf("^iot\\.v1\\.%s\\.%s\\.up\\.", regexpQuote(productID), regexpQuote(deviceID))
	short := "^v1\\."
	// LWT / status + uplinks
	return strings.Join([]string{fullUp, short}, "|")
}

func topicReadACL(productID, deviceID string) string {
	fullDown := fmt.Sprintf("^iot\\.v1\\.%s\\.%s\\.down\\.", regexpQuote(productID), regexpQuote(deviceID))
	short := "^v1\\."
	return strings.Join([]string{fullDown, short}, "|")
}

func regexpQuote(s string) string {
	replacer := strings.NewReplacer(
		`.`, `\.`, `+`, `\+`, `*`, `\*`, `?`, `\?`, `^`, `\^`, `$`, `\$`,
		`(`, `\(`, `)`, `\)`, `[`, `\[`, `]`, `\]`, `{`, `\{`, `}`, `\}`, `|`, `\|`,
	)
	return replacer.Replace(s)
}
