package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"cogitor-iot-platform/internal/db"
	"cogitor-iot-platform/internal/models"
	"cogitor-iot-platform/internal/plugins"
	"cogitor-iot-platform/internal/protocol"
)

type PluginHub struct {
	plat *Platform

	mu      sync.Mutex
	runners map[int64]*pluginRunner
	root    context.Context
	cancel  context.CancelFunc
}

type pluginRunner struct {
	binding   models.DevicePluginBinding
	inst      plugins.Instance
	cancel    context.CancelFunc
	running   bool
	lastErr   string
	accountID int64 // device's account for policy checks
}

type pluginRuntime struct {
	plat *Platform
	d    *models.Device
}

func (r pluginRuntime) InvokeRPC(method string, params map[string]any) error {
	_, err := r.plat.InvokeRPC(r.d, RPCRequest{
		Method:        method,
		Params:        params,
		TimeoutMS:     4000,
		CorrelationID: "plugin",
	}, false)
	return err
}

func (r pluginRuntime) Log() *slog.Logger {
	return r.plat.Log
}

func newPluginHub(p *Platform) *PluginHub {
	return &PluginHub{plat: p, runners: map[int64]*pluginRunner{}}
}

func (h *PluginHub) Start(ctx context.Context) {
	h.mu.Lock()
	if h.cancel != nil {
		h.mu.Unlock()
		return
	}
	h.root, h.cancel = context.WithCancel(ctx)
	h.mu.Unlock()

	h.plat.pruneNonDeviceBindings()
	list, err := h.plat.ListAllPluginBindings()
	if err != nil {
		if h.plat.Log != nil {
			h.plat.Log.Warn("plugin bindings load failed", "err", err)
		}
		return
	}
	for _, b := range list {
		h.syncRunner(b)
	}
}

func (h *PluginHub) Stop() {
	h.mu.Lock()
	if h.cancel != nil {
		h.cancel()
		h.cancel = nil
	}
	for id, r := range h.runners {
		if r.cancel != nil {
			r.cancel()
		}
		delete(h.runners, id)
	}
	h.mu.Unlock()
}

func (h *PluginHub) Dispatch(productID, deviceID, kind string, payload any) {
	d := h.plat.ResolveByClientID(productID + "." + deviceID)
	if d == nil {
		return
	}
	if kind == protocol.BusStatus {
		list, _ := h.plat.ListDevicePlugins(d.ID)
		for _, b := range list {
			h.syncRunner(b)
		}
		h.forward(d, kind, payload)
		return
	}
	h.forward(d, kind, payload)
}

func (h *PluginHub) forward(d *models.Device, kind string, payload any) {
	h.mu.Lock()
	var insts []struct {
		inst plugins.Instance
		b    models.DevicePluginBinding
	}
	for _, r := range h.runners {
		if r.binding.DevicePK == d.ID && r.binding.Enabled && r.inst != nil {
			// Check per-account plugin policy
			if r.accountID > 0 && !h.plat.IsPluginEnabled(r.accountID, r.binding.PluginID) {
				continue
			}
			insts = append(insts, struct {
				inst plugins.Instance
				b    models.DevicePluginBinding
			}{r.inst, r.binding})
		}
	}
	h.mu.Unlock()
	if h.plat.Log != nil && len(insts) > 0 {
		pluginIDs := make([]string, 0, len(insts))
		for _, it := range insts {
			pluginIDs = append(pluginIDs, it.b.PluginID)
		}
		h.plat.Log.Debug("plugin dispatch",
			"kind", kind, "product", d.ProductID, "device", d.DeviceID,
			"plugins", pluginIDs, "device_pk", d.ID)
	}
	rt := pluginRuntime{plat: h.plat, d: d}
	online := d.Status == "online"
	if kind == protocol.BusStatus {
		if m, ok := payload.(map[string]any); ok {
			if v, ok := m["online"].(bool); ok {
				online = v
			}
		}
	}
	ev := plugins.Event{Kind: kind, Online: online, Payload: payload, DevicePK: d.ID}
	for _, it := range insts {
		_ = it.inst.OnEvent(rt, ev)
	}

	// External plugins receive events via MQTT rules bus subscription — no push needed.
}

func (h *PluginHub) Apply(bindingID int64) error {
	b, err := h.plat.GetPluginBinding(bindingID)
	if err != nil {
		return err
	}
	h.syncRunner(*b)
	d, err := h.plat.GetDevice(b.DevicePK)
	if err != nil {
		return err
	}
	h.mu.Lock()
	r := h.runners[bindingID]
	h.mu.Unlock()
	if r == nil || r.inst == nil || !b.Enabled {
		return nil
	}
	if d.Status != "online" {
		return errors.New("device offline")
	}
	return r.inst.OnEvent(pluginRuntime{plat: h.plat, d: d}, plugins.Event{Kind: plugins.EventApply, Online: true, DevicePK: d.ID})
}

func (h *PluginHub) Running(bindingID int64) (bool, string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	r := h.runners[bindingID]
	if r == nil {
		return false, ""
	}
	return r.running, r.lastErr
}

func (h *PluginHub) syncRunner(b models.DevicePluginBinding) {
	h.mu.Lock()
	if old, ok := h.runners[b.ID]; ok && old.cancel != nil {
		old.cancel()
	}
	delete(h.runners, b.ID)
	if h.root == nil || !b.Enabled {
		h.mu.Unlock()
		return
	}
	_, fac, ok := plugins.Lookup(b.PluginID)
	if !ok || fac == nil {
		h.mu.Unlock()
		return
	}
	// Check per-account plugin policy before creating runner
	d, err := h.plat.GetDevice(b.DevicePK)
	if err == nil && d != nil && d.AccountID > 0 && !h.plat.IsPluginEnabled(d.AccountID, b.PluginID) {
		h.mu.Unlock()
		return
	}
	inst := fac()
	if err := inst.ApplyConfig(b.Config); err != nil {
		h.mu.Unlock()
		if h.plat.Log != nil {
			h.plat.Log.Warn("plugin config", "plugin", b.PluginID, "err", err)
		}
		return
	}
	online := err == nil && d != nil && d.Status == "online" && d.Enabled
	var accountID int64
	if d != nil {
		accountID = d.AccountID
	}
	r := &pluginRunner{binding: b, inst: inst, accountID: accountID}
	h.runners[b.ID] = r
	var (
		start    bool
		interval time.Duration
		ctx      context.Context
	)
	if online && inst.Auto() {
		var cancel context.CancelFunc
		ctx, cancel = context.WithCancel(h.root)
		r.cancel = cancel
		r.running = true
		interval = time.Duration(inst.IntervalMS()) * time.Millisecond
		if interval < 80*time.Millisecond {
			interval = 200 * time.Millisecond
		}
		start = true
	}
	h.mu.Unlock()
	if start {
		go h.tickLoop(ctx, b.ID, interval)
	}
}

func (h *PluginHub) tickLoop(ctx context.Context, bindingID int64, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	h.tickOnce(bindingID)
	for {
		select {
		case <-ctx.Done():
			h.mu.Lock()
			if r := h.runners[bindingID]; r != nil {
				r.running = false
			}
			h.mu.Unlock()
			return
		case <-t.C:
			h.tickOnce(bindingID)
		}
	}
}

func (h *PluginHub) tickOnce(bindingID int64) {
	h.mu.Lock()
	r := h.runners[bindingID]
	h.mu.Unlock()
	if r == nil || r.inst == nil || !r.binding.Enabled {
		return
	}
	// Check per-account plugin policy
	if r.accountID > 0 && !h.plat.IsPluginEnabled(r.accountID, r.binding.PluginID) {
		return
	}
	d, err := h.plat.GetDevice(r.binding.DevicePK)
	if err != nil || d.Status != "online" || !d.Enabled {
		return
	}
	err = r.inst.OnEvent(pluginRuntime{plat: h.plat, d: d}, plugins.Event{Kind: plugins.EventTick, Online: true, DevicePK: d.ID})
	h.mu.Lock()
	if rr := h.runners[bindingID]; rr != nil {
		if err != nil {
			rr.lastErr = err.Error()
		} else {
			rr.lastErr = ""
		}
	}
	h.mu.Unlock()
}

func (p *Platform) StartPlugins(ctx context.Context) {
	if p.Plugins == nil {
		p.Plugins = newPluginHub(p)
	}
	p.Plugins.Start(ctx)
	if p.ExternalPlugins == nil {
		p.ExternalPlugins = newExternalPluginHub(p)
	}
	p.ExternalPlugins.Start(ctx)
}

// RebuildAccountRunners re-syncs all plugin runners for devices belonging to
// the given account. Called after a plugin policy change so that enable/disable
// takes effect immediately.
func (h *PluginHub) RebuildAccountRunners(accountID int64) {
	list, err := h.plat.ListAllPluginBindings()
	if err != nil {
		if h.plat.Log != nil {
			h.plat.Log.Warn("rebuild account runners: list failed", "account", accountID, "err", err)
		}
		return
	}
	for _, b := range list {
		d, err := h.plat.GetDevice(b.DevicePK)
		if err != nil || d == nil || d.AccountID != accountID {
			continue
		}
		h.syncRunner(b)
	}
}

func (p *Platform) ListPluginCatalog() []models.PluginInfo {
	cat := plugins.Catalog()
	out := make([]models.PluginInfo, 0, len(cat))
	for _, d := range cat {
		out = append(out, models.PluginInfo{
			ID: d.ID, Name: d.Name, Description: d.Description,
			DefaultConfig: d.DefaultConfig, Effects: d.Effects,
			Form: plugins.FormJSON(d.Form),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// pruneNonDeviceBindings removes device bindings of plugins that are not
// device plugins — e.g. external plugins such as Node-RED, which earlier
// versions let users attach to a device although they never run per device.
func (p *Platform) pruneNonDeviceBindings() {
	list, err := p.ListAllPluginBindings()
	if err != nil {
		return
	}
	for _, b := range list {
		if _, _, ok := plugins.Lookup(b.PluginID); ok {
			continue
		}
		if _, err := p.DB.Exec(`DELETE FROM device_plugin_bindings WHERE id=?`, b.ID); err == nil && p.Log != nil {
			p.Log.Info("removed device binding of a non-device plugin", "plugin", b.PluginID, "device_pk", b.DevicePK)
		}
	}
}

func (p *Platform) ListAllPluginBindings() ([]models.DevicePluginBinding, error) {
	rows, err := p.DB.Query(`SELECT id, device_pk, plugin_id, enabled, config_json, created_at, updated_at FROM device_plugin_bindings ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPluginBindings(rows)
}

func (p *Platform) ListDevicePlugins(devicePK int64) ([]models.DevicePluginBinding, error) {
	if _, err := p.GetDevice(devicePK); err != nil {
		return nil, err
	}
	rows, err := p.DB.Query(`SELECT id, device_pk, plugin_id, enabled, config_json, created_at, updated_at FROM device_plugin_bindings WHERE device_pk=? ORDER BY id`, devicePK)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list, err := scanPluginBindings(rows)
	if err != nil {
		return nil, err
	}
	for i := range list {
		p.decorateBinding(&list[i])
	}
	return list, nil
}

func (p *Platform) GetPluginBinding(id int64) (*models.DevicePluginBinding, error) {
	row := p.DB.QueryRow(`SELECT id, device_pk, plugin_id, enabled, config_json, created_at, updated_at FROM device_plugin_bindings WHERE id=?`, id)
	b, err := scanPluginBinding(row)
	if err != nil {
		return nil, errors.New("plugin binding not found")
	}
	p.decorateBinding(&b)
	return &b, nil
}

func (p *Platform) decorateBinding(b *models.DevicePluginBinding) {
	if d, _, ok := plugins.Lookup(b.PluginID); ok {
		b.Name, b.Description, b.Effects = d.Name, d.Description, d.Effects
		b.Form = plugins.FormJSON(d.Form)
	}
	if p.Plugins != nil {
		b.Running, b.LastError = p.Plugins.Running(b.ID)
	}
}

type PluginBindingInput struct {
	PluginID string          `json:"plugin_id"`
	Enabled  *bool           `json:"enabled"`
	Config   json.RawMessage `json:"config"`
}

func (p *Platform) BindDevicePlugin(devicePK int64, in PluginBindingInput) (*models.DevicePluginBinding, error) {
	device, err := p.GetDevice(devicePK)
	if err != nil {
		return nil, err
	}
	in.PluginID = strings.TrimSpace(in.PluginID)

	desc, fac, ok := plugins.Lookup(in.PluginID)
	if !ok {
		if p.ExternalPlugins != nil {
			if ep, ext := p.ExternalPlugins.GetPlugin(in.PluginID); ext && ep.AccountID == device.AccountID {
				return nil, errors.New("external plugins work account-wide over the rules bus and are not bound to devices")
			}
		}
		return nil, errors.New("unknown plugin")
	}

	cfg := in.Config
	if len(cfg) == 0 || string(cfg) == "null" {
		cfg = desc.DefaultConfig
	}
	if fac != nil {
		if err := fac().ApplyConfig(cfg); err != nil {
			return nil, err
		}
	}
	enabled := 1
	if in.Enabled != nil && !*in.Enabled {
		enabled = 0
	}
	now := db.NowMS()
	res, err := p.DB.Exec(`INSERT INTO device_plugin_bindings(device_pk, plugin_id, enabled, config_json, created_at, updated_at) VALUES(?,?,?,?,?,?)`,
		devicePK, in.PluginID, enabled, string(cfg), now, now)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return nil, errors.New("plugin already bound to this device")
		}
		return nil, err
	}
	id, _ := res.LastInsertId()
	b, err := p.GetPluginBinding(id)
	if err != nil {
		return nil, err
	}
	if p.Plugins != nil {
		p.Plugins.syncRunner(*b)
		if enabled == 1 {
			_ = p.Plugins.Apply(b.ID)
		}
	}
	if fresh, err := p.GetPluginBinding(id); err == nil {
		b = fresh
	}
	return b, nil
}

func (p *Platform) UpdatePluginBinding(id int64, in PluginBindingInput) (*models.DevicePluginBinding, error) {
	b, err := p.GetPluginBinding(id)
	if err != nil {
		return nil, err
	}
	cfg := b.Config
	_, fac, isInternal := plugins.Lookup(b.PluginID)
	if len(in.Config) > 0 && string(in.Config) != "null" {
		if isInternal && fac != nil {
			if err := fac().ApplyConfig(in.Config); err != nil {
				return nil, err
			}
		}
		cfg = in.Config
	}
	enabled := b.Enabled
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	en := 0
	if enabled {
		en = 1
	}
	now := db.NowMS()
	if _, err := p.DB.Exec(`UPDATE device_plugin_bindings SET enabled=?, config_json=?, updated_at=? WHERE id=?`, en, string(cfg), now, id); err != nil {
		return nil, err
	}
	b, err = p.GetPluginBinding(id)
	if err != nil {
		return nil, err
	}
	// Internal plugin: sync runner
	if isInternal && p.Plugins != nil {
		p.Plugins.syncRunner(*b)
		if b.Enabled {
			_ = p.Plugins.Apply(b.ID)
		}
	}
	// External plugin: config updated in DB; plugin picks up via MQTT.
	if fresh, err := p.GetPluginBinding(id); err == nil {
		b = fresh
	}
	return b, nil
}

func (p *Platform) DeletePluginBinding(id int64) error {
	b, err := p.GetPluginBinding(id)
	if err != nil {
		return err
	}
	// External plugins are notified of detach via MQTT topic unsubscribe.
	if _, err := p.DB.Exec(`DELETE FROM device_plugin_bindings WHERE id=?`, id); err != nil {
		return err
	}
	if p.Plugins != nil {
		b.Enabled = false
		p.Plugins.syncRunner(*b)
	}
	return nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanPluginBinding(s scanner) (models.DevicePluginBinding, error) {
	var b models.DevicePluginBinding
	var enabled int
	var cfg string
	err := s.Scan(&b.ID, &b.DevicePK, &b.PluginID, &enabled, &cfg, &b.CreatedAt, &b.UpdatedAt)
	if err != nil {
		return b, err
	}
	b.Enabled = enabled != 0
	b.Config = json.RawMessage(cfg)
	if len(b.Config) == 0 {
		b.Config = json.RawMessage(`{}`)
	}
	return b, nil
}

func scanPluginBindings(rows *sql.Rows) ([]models.DevicePluginBinding, error) {
	var out []models.DevicePluginBinding
	for rows.Next() {
		b, err := scanPluginBinding(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	if out == nil {
		out = []models.DevicePluginBinding{}
	}
	return out, nil
}
