package services

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"sync"

	"cogitor-iot-platform/internal/db"
	"cogitor-iot-platform/internal/models"
)

// pluginIDRe keeps the id safe inside the MQTT username extplg_{slug}_{id}.
var pluginIDRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,63}$`)

// ExternalPluginHub manages out-of-process plugins that connect via MQTT (or AMQP in cluster mode).
type ExternalPluginHub struct {
	plat *Platform

	mu      sync.RWMutex
	plugins map[string]*models.ExternalPlugin // id → plugin
}

func newExternalPluginHub(p *Platform) *ExternalPluginHub {
	return &ExternalPluginHub{
		plat:    p,
		plugins: map[string]*models.ExternalPlugin{},
	}
}

// Start loads all external plugins from DB.
func (h *ExternalPluginHub) Start(_ context.Context) {
	list, err := h.plat.ListExternalPlugins()
	if err != nil {
		if h.plat.Log != nil {
			h.plat.Log.Error("external plugins load failed", "err", err)
		}
		return
	}
	h.mu.Lock()
	for i := range list {
		h.plugins[list[i].ID] = &list[i]
	}
	h.mu.Unlock()

	if h.plat.Log != nil {
		h.plat.Log.Info("external plugins loaded", "count", len(list))
	}
}

// Register adds or updates an external plugin.
// Returns plugin and MQTT password (only on first registration), error.
func (h *ExternalPluginHub) Register(in RegisterExternalPluginInput) (*models.ExternalPlugin, string, error) {
	if h.plat.Log != nil {
		h.plat.Log.Info("external plugin register request",
			"id", in.ID, "account_id", in.AccountID, "account_slug", in.AccountSlug,
			"name", in.Name, "auto", in.Auto, "interval_ms", in.IntervalMS)
	}
	if in.ID == "" {
		return nil, "", errors.New("plugin id required")
	}
	if !pluginIDRe.MatchString(in.ID) {
		return nil, "", errors.New("invalid plugin id: 1-64 chars [a-zA-Z0-9._-], starting with a letter or digit")
	}
	if in.Name == "" {
		return nil, "", errors.New("plugin name required")
	}
	if existing, ok := h.GetPlugin(in.ID); ok && existing.AccountID != in.AccountID {
		return nil, "", errors.New("plugin id already taken")
	}

	// Generate MQTT credentials — username includes account slug for ACL scoping
	mqttUser := "extplg_" + in.AccountSlug + "_" + in.ID
	mqttPass := generateToken(16)
	mqttPassHash := hashToken(mqttPass)

	now := db.NowMS()
	p := &models.ExternalPlugin{
		ID:            in.ID,
		AccountID:     in.AccountID,
		Name:          in.Name,
		Description:   in.Description,
		Auto:          in.Auto,
		IntervalMS:    in.IntervalMS,
		DefaultConfig: in.DefaultConfig,
		Effects:       in.Effects,
		Form:          in.Form,
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
		return nil, "", err
	}

	h.mu.Lock()
	h.plugins[p.ID] = p
	h.mu.Unlock()

	if h.plat.Log != nil {
		h.plat.Log.Info("external plugin registered",
			"id", p.ID, "account_id", p.AccountID, "mqtt_user", p.MQTTUser)
	}
	return p, mqttPass, nil
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

// ListForAccount returns the external plugins registered in accountID, sorted by ID.
func (h *ExternalPluginHub) ListForAccount(accountID int64) []models.ExternalPlugin {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]models.ExternalPlugin, 0)
	for _, p := range h.plugins {
		if p.AccountID == accountID {
			out = append(out, *p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// OwnedBy reports whether plugin id exists and belongs to accountID.
func (h *ExternalPluginHub) OwnedBy(id string, accountID int64) bool {
	p, ok := h.GetPlugin(id)
	return ok && p.AccountID == accountID
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
			if h.plat.Log != nil {
				h.plat.Log.Debug("external plugin MQTT auth ok", "id", p.ID, "user", user)
			}
			return p.ID, true
		}
	}
	if h.plat.Log != nil {
		h.plat.Log.Debug("external plugin MQTT auth failed", "user", user)
	}
	return "", false
}

// Delete removes an external plugin.
func (h *ExternalPluginHub) Delete(id string) error {
	if h.plat.Log != nil {
		h.plat.Log.Debug("external plugin delete request", "id", id)
	}
	if err := h.plat.deleteExternalPlugin(id); err != nil {
		return err
	}
	h.mu.Lock()
	delete(h.plugins, id)
	h.mu.Unlock()
	if h.plat.Log != nil {
		h.plat.Log.Info("external plugin deleted", "id", id)
	}
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

// SetOnline updates the plugin status to "online".
func (h *ExternalPluginHub) SetOnline(id string) {
	h.mu.Lock()
	if p, ok := h.plugins[id]; ok {
		p.Status = "online"
	}
	h.mu.Unlock()
	_ = h.plat.updateExternalPluginStatus(id, "online")
	if h.plat.Log != nil {
		h.plat.Log.Info("external plugin online", "id", id)
	}
}

// SetOffline updates the plugin status to "offline".
func (h *ExternalPluginHub) SetOffline(id string) {
	h.mu.Lock()
	if p, ok := h.plugins[id]; ok {
		p.Status = "offline"
	}
	h.mu.Unlock()
	_ = h.plat.updateExternalPluginStatus(id, "offline")
	if h.plat.Log != nil {
		h.plat.Log.Info("external plugin offline", "id", id)
	}
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

	// Cross-account check: plugin may only control devices in its own account
	if p.ExternalPlugins != nil {
		if plugin, ok := p.ExternalPlugins.GetPlugin(pluginID); ok && plugin.AccountID > 0 && device.AccountID > 0 {
			if plugin.AccountID != device.AccountID {
				return errors.New("cross-account RPC denied")
			}
		}
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

// Helper types

type RegisterExternalPluginInput struct {
	ID            string          `json:"id"`
	AccountID     int64           `json:"account_id"`
	AccountSlug   string          `json:"account_slug"`
	Name          string          `json:"name"`
	Description   string          `json:"description"`
	Auto          bool            `json:"auto"`
	IntervalMS    int             `json:"interval_ms"`
	DefaultConfig json.RawMessage `json:"default_config"`
	Effects       []string        `json:"effects"`
	Form          json.RawMessage `json:"form"`
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
	rows, err := p.DB.Query(`SELECT id, COALESCE(account_id,0), name, description, auto, interval_ms, 
		default_config, effects, form, status, mqtt_user, mqtt_pass_hash, created_at, updated_at 
		FROM external_plugins ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.ExternalPlugin
	for rows.Next() {
		var ep models.ExternalPlugin
		var autoInt int
		var configStr, effectsStr, formStr string
		err := rows.Scan(&ep.ID, &ep.AccountID, &ep.Name, &ep.Description,
			&autoInt, &ep.IntervalMS, &configStr, &effectsStr, &formStr,
			&ep.Status, &ep.MQTTUser, &ep.MQTTPassHash, &ep.CreatedAt, &ep.UpdatedAt)
		if err != nil {
			return nil, err
		}
		ep.Auto = autoInt != 0
		ep.DefaultConfig = json.RawMessage(configStr)
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
	_, err := p.DB.Exec(`INSERT INTO external_plugins 
		(id, account_id, name, description, auto, interval_ms, default_config, effects, form, status, mqtt_user, mqtt_pass_hash, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET 
			account_id=excluded.account_id, name=excluded.name, description=excluded.description,
			auto=excluded.auto, interval_ms=excluded.interval_ms, default_config=excluded.default_config,
			effects=excluded.effects, form=excluded.form, status=excluded.status,
			mqtt_user=excluded.mqtt_user, mqtt_pass_hash=excluded.mqtt_pass_hash, updated_at=excluded.updated_at`,
		ep.ID, ep.AccountID, ep.Name, ep.Description, autoInt, ep.IntervalMS,
		string(ep.DefaultConfig), string(effectsJSON), string(ep.Form), ep.Status,
		ep.MQTTUser, ep.MQTTPassHash, ep.CreatedAt, ep.UpdatedAt)
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

func (p *Platform) updateExternalPluginStatus(id, status string) error {
	_, err := p.DB.Exec(`UPDATE external_plugins SET status = ?, updated_at = ? WHERE id = ?`,
		status, db.NowMS(), id)
	return err
}
