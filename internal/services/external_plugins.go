package services

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"crearts-iot-platform/internal/db"
	"crearts-iot-platform/internal/models"
)

// ExternalPluginHub manages out-of-process plugins.
type ExternalPluginHub struct {
	plat *Platform

	mu      sync.RWMutex
	plugins map[string]*models.ExternalPlugin // id → plugin
	tokens  map[string]string                 // token → plugin_id

	httpClient *http.Client
	grpc       *grpcPool
}

func newExternalPluginHub(p *Platform) *ExternalPluginHub {
	return &ExternalPluginHub{
		plat:    p,
		plugins: map[string]*models.ExternalPlugin{},
		tokens:  map[string]string{},
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
		grpc: newGRPCPool(),
	}
}

// Start loads all external plugins from DB and starts heartbeat monitor.
func (h *ExternalPluginHub) Start(ctx context.Context) {
	list, err := h.plat.ListExternalPlugins()
	if err != nil {
		if h.plat.Log != nil {
			h.plat.Log.Warn("external plugins load failed", "err", err)
		}
		return
	}
	h.mu.Lock()
	for i := range list {
		h.plugins[list[i].ID] = &list[i]
	}
	h.mu.Unlock()

	go h.heartbeatMonitor(ctx)
}

// heartbeatMonitor checks for plugins without heartbeat and marks them offline.
func (h *ExternalPluginHub) heartbeatMonitor(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.checkHeartbeats()
		}
	}
}

func (h *ExternalPluginHub) checkHeartbeats() {
	h.mu.RLock()
	defer h.mu.RUnlock()

	ctx := context.Background()
	threshold := time.Now().Add(-90 * time.Second).UnixMilli()
	for _, p := range h.plugins {
		// gRPC plugins — check via Ping
		if isGRPCEndpoint(p.Endpoint) {
			client, err := h.grpc.get(p.ID, p.Endpoint)
			if err != nil {
				if p.Status != "offline" {
					_ = h.plat.updateExternalPluginStatus(p.ID, "offline")
					p.Status = "offline"
				}
				continue
			}
			resp, err := client.ping(ctx)
			if err != nil || !resp.Ok {
				if p.Status != "offline" {
					_ = h.plat.updateExternalPluginStatus(p.ID, "offline")
					p.Status = "offline"
				}
			} else {
				now := time.Now().UnixMilli()
				p.LastHeartbeatAt = &now
				if p.Status != "online" {
					p.Status = "online"
					_ = h.plat.updateExternalPluginHeartbeat(p.ID, now)
				}
			}
			continue
		}

		// webhook/polling plugins — check heartbeat timestamp
		if p.Status == "online" && p.LastHeartbeatAt != nil && *p.LastHeartbeatAt < threshold {
			_ = h.plat.updateExternalPluginStatus(p.ID, "offline")
			p.Status = "offline"
		}
	}
}

// Register adds or updates an external plugin.
// Returns plugin, API token, MQTT password (only on first registration), error.
func (h *ExternalPluginHub) Register(in RegisterExternalPluginInput) (*models.ExternalPlugin, string, string, error) {
	// Validate
	if in.ID == "" {
		return nil, "", "", errors.New("plugin id required")
	}
	if in.Name == "" {
		return nil, "", "", errors.New("plugin name required")
	}

	// Determine mode
	mode := "webhook"
	if in.Endpoint == "" {
		mode = "polling"
	}

	// Generate API token
	token := generateToken(32)
	tokenHash := hashToken(token)

	// Generate MQTT credentials
	mqttUser := "extplg_" + in.ID
	mqttPass := generateToken(16)
	mqttPassHash := hashToken(mqttPass)

	now := db.NowMS()
	p := &models.ExternalPlugin{
		ID:            in.ID,
		Name:          in.Name,
		Description:   in.Description,
		Endpoint:      in.Endpoint,
		AuthTokenHash: tokenHash,
		Auto:          in.Auto,
		IntervalMS:    in.IntervalMS,
		DefaultConfig: in.DefaultConfig,
		Effects:       in.Effects,
		Form:          in.Form,
		Mode:          mode,
		Status:        "unknown",
		MQTTUser:      mqttUser,
		MQTTPassHash:  mqttPassHash,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	if len(p.DefaultConfig) == 0 {
		p.DefaultConfig = json.RawMessage(`{}`)
	}
	if len(p.Form) == 0 {
		p.Form = json.RawMessage(`[]`)
	}
	if len(p.Effects) == 0 {
		p.Effects = []string{}
	}

	// Upsert
	err := h.plat.saveExternalPlugin(p)
	if err != nil {
		return nil, "", "", err
	}

	h.mu.Lock()
	h.plugins[p.ID] = p
	h.tokens[token] = p.ID
	h.mu.Unlock()

	return p, token, mqttPass, nil
}

// Heartbeat updates plugin status and last heartbeat time.
func (h *ExternalPluginHub) Heartbeat(pluginID string, stats HeartbeatStats) error {
	h.mu.Lock()
	p, ok := h.plugins[pluginID]
	if !ok {
		h.mu.Unlock()
		return errors.New("plugin not found")
	}
	now := time.Now().UnixMilli()
	p.LastHeartbeatAt = &now
	p.Status = "online"
	h.mu.Unlock()

	return h.plat.updateExternalPluginHeartbeat(pluginID, now)
}

// Dispatch sends an event to an external plugin by binding.
func (h *ExternalPluginHub) Dispatch(bindingID int64, method string, params any) {
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return
	}

	// Get binding to find plugin ID
	binding, err := h.plat.GetPluginBinding(bindingID)
	if err != nil {
		if h.plat.Log != nil {
			h.plat.Log.Warn("external dispatch: binding not found", "binding_id", bindingID, "err", err)
		}
		return
	}

	h.mu.RLock()
	plugin, ok := h.plugins[binding.PluginID]
	h.mu.RUnlock()
	if !ok {
		return // not an external plugin
	}

	if h.plat.Log != nil {
		h.plat.Log.Debug("external dispatch", "plugin", plugin.ID, "method", method, "mode", plugin.Mode)
	}

	if isGRPCEndpoint(plugin.Endpoint) {
		// Send via gRPC
		go h.sendGRPC(plugin, bindingID, method, paramsJSON)
	} else if plugin.Mode == "webhook" && plugin.Endpoint != "" {
		// Send via HTTP webhook
		go h.sendWebhook(plugin, bindingID, method, paramsJSON)
	} else {
		// Queue for polling
		_ = h.plat.queueExternalPluginEvent(plugin.ID, bindingID, method, paramsJSON)
	}
}

func (h *ExternalPluginHub) sendWebhook(plugin *models.ExternalPlugin, bindingID int64, method string, params json.RawMessage) {
	payload := map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  json.RawMessage(params),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return
	}

	if h.plat.Log != nil {
		h.plat.Log.Debug("webhook send", "plugin", plugin.ID, "url", plugin.Endpoint, "method", method)
	}

	req, err := http.NewRequest("POST", plugin.Endpoint, bytes.NewReader(body))
	if err != nil {
		if h.plat.Log != nil {
			h.plat.Log.Warn("webhook request build failed", "plugin", plugin.ID, "err", err)
		}
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := h.httpClient.Do(req)
	if err != nil {
		if h.plat.Log != nil {
			h.plat.Log.Warn("webhook failed", "plugin", plugin.ID, "url", plugin.Endpoint, "err", err)
		}
		return
	}
	defer resp.Body.Close()
	if h.plat.Log != nil && resp.StatusCode >= 400 {
		h.plat.Log.Warn("webhook error response", "plugin", plugin.ID, "status", resp.StatusCode)
	}
}

// isGRPCEndpoint returns true if endpoint is a gRPC address (not HTTP).
func isGRPCEndpoint(endpoint string) bool {
	if endpoint == "" {
		return false
	}
	return !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://")
}

// sendGRPC sends an RPC call to a gRPC plugin.
func (h *ExternalPluginHub) sendGRPC(plugin *models.ExternalPlugin, bindingID int64, method string, params json.RawMessage) {
	ctx := context.Background()

	client, err := h.grpc.get(plugin.ID, plugin.Endpoint)
	if err != nil {
		if h.plat.Log != nil {
			h.plat.Log.Warn("grpc connect failed", "plugin", plugin.ID, "endpoint", plugin.Endpoint, "err", err)
		}
		return
	}

	// Parse params to extract fields for gRPC call
	var p struct {
		BindingID int64           `json:"binding_id"`
		DevicePK  int64           `json:"device_pk"`
		DeviceID  string          `json:"device_id"`
		ProductID string          `json:"product_id"`
		Kind      string          `json:"kind"`
		Payload   json.RawMessage `json:"payload"`
		Online    bool            `json:"online"`
		Config    json.RawMessage `json:"config"`
	}
	_ = json.Unmarshal(params, &p)

	if h.plat.Log != nil {
		h.plat.Log.Debug("grpc send", "plugin", plugin.ID, "endpoint", plugin.Endpoint, "method", method)
	}

	switch method {
	case "plugin.on_event":
		err = client.onEvent(ctx, p.BindingID, p.DevicePK, p.DeviceID, p.ProductID, p.Kind, p.Payload, p.Online)
	case "plugin.apply_config":
		err = client.applyConfig(ctx, p.BindingID, p.DevicePK, p.DeviceID, p.ProductID, p.Config)
	case "plugin.detach":
		err = client.detach(ctx, p.BindingID, p.DevicePK)
	default:
		if h.plat.Log != nil {
			h.plat.Log.Warn("grpc unknown method", "plugin", plugin.ID, "method", method)
		}
		return
	}

	if err != nil {
		if h.plat.Log != nil {
			h.plat.Log.Warn("grpc call failed", "plugin", plugin.ID, "method", method, "err", err)
		}
	}
}

// PollEvents returns queued events for a plugin (polling mode).
func (h *ExternalPluginHub) PollEvents(pluginID string, limit int) ([]models.ExternalPluginEvent, error) {
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	return h.plat.getExternalPluginEvents(pluginID, limit)
}

// AckEvents marks events as acknowledged.
func (h *ExternalPluginHub) AckEvents(pluginID string, eventIDs []int64) error {
	return h.plat.ackExternalPluginEvents(eventIDs)
}

// GetPlugin returns a plugin by ID.
func (h *ExternalPluginHub) GetPlugin(id string) (*models.ExternalPlugin, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	p, ok := h.plugins[id]
	return p, ok
}

// ListAll returns all registered external plugins.
func (h *ExternalPluginHub) ListAll() []models.ExternalPlugin {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]models.ExternalPlugin, 0, len(h.plugins))
	for _, p := range h.plugins {
		out = append(out, *p)
	}
	return out
}

// ValidateToken checks if a token matches the plugin's stored hash.
func (h *ExternalPluginHub) ValidateToken(pluginID, token string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	storedID, ok := h.tokens[token]
	return ok && storedID == pluginID
}

// GetPluginIDByToken returns the plugin ID for a given token.
func (h *ExternalPluginHub) GetPluginIDByToken(token string) (string, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	id, ok := h.tokens[token]
	return id, ok
}

// ValidateMQTTCreds checks MQTT username/password for external plugin auth.
// Returns plugin ID if valid.
func (h *ExternalPluginHub) ValidateMQTTCreds(user, pass string) (string, bool) {
	if user == "" || pass == "" {
		return "", false
	}
	passHash := hashToken(pass)
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, p := range h.plugins {
		if p.MQTTUser == user && p.MQTTPassHash == passHash {
			return p.ID, true
		}
	}
	return "", false
}

// Delete removes an external plugin.
func (h *ExternalPluginHub) Delete(id string) error {
	if err := h.plat.deleteExternalPlugin(id); err != nil {
		return err
	}
	h.mu.Lock()
	delete(h.plugins, id)
	h.mu.Unlock()
	return nil
}

// RegenerateMQTTPassword generates a new MQTT password for a plugin.
// Returns the new plaintext password (only time it's available).
func (h *ExternalPluginHub) RegenerateMQTTPassword(id string) (string, error) {
	h.mu.Lock()
	p, ok := h.plugins[id]
	if !ok {
		h.mu.Unlock()
		return "", errors.New("plugin not found")
	}
	newPass := generateToken(16)
	p.MQTTPassHash = hashToken(newPass)
	h.mu.Unlock()

	if err := h.plat.updateExternalPluginMQTTPass(id, p.MQTTPassHash); err != nil {
		return "", err
	}
	return newPass, nil
}

// Helper types

type RegisterExternalPluginInput struct {
	ID            string          `json:"id"`
	Name          string          `json:"name"`
	Description   string          `json:"description"`
	Endpoint      string          `json:"endpoint"`
	Auto          bool            `json:"auto"`
	IntervalMS    int             `json:"interval_ms"`
	DefaultConfig json.RawMessage `json:"default_config"`
	Effects       []string        `json:"effects"`
	Form          json.RawMessage `json:"form"`
}

type HeartbeatStats struct {
	EventsProcessed int `json:"events_processed"`
	Errors          int `json:"errors"`
	UptimeSec       int `json:"uptime_sec"`
}

// Helper functions

func generateToken(length int) string {
	b := make([]byte, length)
	_, _ = rand.Read(b)
	return "ext_plg_" + hex.EncodeToString(b)
}

func hashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

// Platform methods for external plugins

func (p *Platform) ListExternalPlugins() ([]models.ExternalPlugin, error) {
	rows, err := p.DB.Query(`SELECT id, name, description, endpoint, auth_token_hash, auto, interval_ms, 
		default_config, effects, form, mode, status, last_heartbeat_at, mqtt_user, mqtt_pass_hash, created_at, updated_at 
		FROM external_plugins ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.ExternalPlugin
	for rows.Next() {
		var ep models.ExternalPlugin
		var autoInt int
		var effectsStr, formStr string
		var lastHB *int64
		err := rows.Scan(&ep.ID, &ep.Name, &ep.Description, &ep.Endpoint, &ep.AuthTokenHash,
			&autoInt, &ep.IntervalMS, &ep.DefaultConfig, &effectsStr, &formStr,
			&ep.Mode, &ep.Status, &lastHB, &ep.MQTTUser, &ep.MQTTPassHash, &ep.CreatedAt, &ep.UpdatedAt)
		if err != nil {
			return nil, err
		}
		ep.Auto = autoInt != 0
		ep.LastHeartbeatAt = lastHB
		_ = json.Unmarshal([]byte(effectsStr), &ep.Effects)
		ep.Form = json.RawMessage(formStr)
		out = append(out, ep)
	}
	return out, nil
}

func (p *Platform) saveExternalPlugin(ep *models.ExternalPlugin) error {
	effectsJSON, _ := json.Marshal(ep.Effects)
	autoInt := 0
	if ep.Auto {
		autoInt = 1
	}
	_, err := p.DB.Exec(`INSERT OR REPLACE INTO external_plugins 
		(id, name, description, endpoint, auth_token_hash, auto, interval_ms, default_config, effects, form, mode, status, last_heartbeat_at, mqtt_user, mqtt_pass_hash, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ep.ID, ep.Name, ep.Description, ep.Endpoint, ep.AuthTokenHash, autoInt, ep.IntervalMS,
		string(ep.DefaultConfig), string(effectsJSON), string(ep.Form), ep.Mode, ep.Status,
		ep.LastHeartbeatAt, ep.MQTTUser, ep.MQTTPassHash, ep.CreatedAt, ep.UpdatedAt)
	return err
}

func (p *Platform) updateExternalPluginStatus(id, status string) error {
	_, err := p.DB.Exec(`UPDATE external_plugins SET status = ?, updated_at = ? WHERE id = ?`,
		status, db.NowMS(), id)
	return err
}

func (p *Platform) updateExternalPluginHeartbeat(id string, ts int64) error {
	_, err := p.DB.Exec(`UPDATE external_plugins SET last_heartbeat_at = ?, status = 'online', updated_at = ? WHERE id = ?`,
		ts, db.NowMS(), id)
	return err
}

func (p *Platform) deleteExternalPlugin(id string) error {
	_, err := p.DB.Exec(`DELETE FROM external_plugins WHERE id = ?`, id)
	return err
}

func (p *Platform) updateExternalPluginMQTTPass(id, passHash string) error {
	_, err := p.DB.Exec(`UPDATE external_plugins SET mqtt_pass_hash = ?, updated_at = ? WHERE id = ?`,
		passHash, db.NowMS(), id)
	return err
}

func (p *Platform) queueExternalPluginEvent(pluginID string, bindingID int64, method string, params json.RawMessage) error {
	_, err := p.DB.Exec(`INSERT INTO external_plugin_events (plugin_id, binding_id, method, params, created_at)
		VALUES (?, ?, ?, ?, ?)`, pluginID, bindingID, method, string(params), db.NowMS())
	return err
}

func (p *Platform) getExternalPluginEvents(pluginID string, limit int) ([]models.ExternalPluginEvent, error) {
	rows, err := p.DB.Query(`SELECT id, plugin_id, binding_id, method, params, created_at, acked_at
		FROM external_plugin_events WHERE plugin_id = ? AND acked_at IS NULL ORDER BY id LIMIT ?`,
		pluginID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.ExternalPluginEvent
	for rows.Next() {
		var ev models.ExternalPluginEvent
		var paramsStr string
		err := rows.Scan(&ev.ID, &ev.PluginID, &ev.BindingID, &ev.Method, &paramsStr, &ev.CreatedAt, &ev.AckedAt)
		if err != nil {
			return nil, err
		}
		ev.Params = json.RawMessage(paramsStr)
		out = append(out, ev)
	}
	return out, nil
}

func (p *Platform) ackExternalPluginEvents(ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	now := db.NowMS()
	for _, id := range ids {
		_, err := p.DB.Exec(`UPDATE external_plugin_events SET acked_at = ? WHERE id = ?`, now, id)
		if err != nil {
			return err
		}
	}
	return nil
}

// InvokeRPCForPlugin allows an external plugin to invoke RPC on its bound device.
func (p *Platform) InvokeRPCForPlugin(pluginID string, bindingID int64, method string, params map[string]any, timeoutMS int) error {
	// Verify binding belongs to this plugin
	binding, err := p.GetPluginBinding(bindingID)
	if err != nil {
		return err
	}
	if binding.PluginID != pluginID {
		return errors.New("binding does not belong to this plugin")
	}

	// Get device
	device, err := p.GetDevice(binding.DevicePK)
	if err != nil {
		return err
	}

	// Invoke RPC
	_, err = p.InvokeRPC(device, RPCRequest{
		Method:        method,
		Params:        params,
		TimeoutMS:     timeoutMS,
		CorrelationID: fmt.Sprintf("ext_plugin_%s", pluginID),
	}, false)
	return err
}
