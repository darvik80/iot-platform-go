package services

import (
	"database/sql"
	"encoding/json"
	"errors"

	"cogitor-iot-platform/internal/db"
	"cogitor-iot-platform/internal/models"
	"cogitor-iot-platform/internal/plugins"
)

// GetPluginPolicy returns the effective policy for a catalog plugin in an account.
// If no row exists the plugin is considered enabled with empty settings.
func (p *Platform) GetPluginPolicy(accountID int64, pluginID string) (models.AccountPluginPolicy, error) {
	row := p.DB.QueryRow(
		`SELECT account_id, plugin_id, enabled, settings_json, updated_at
		 FROM account_plugin_policies WHERE account_id=? AND plugin_id=?`,
		accountID, pluginID)
	var pol models.AccountPluginPolicy
	var enabled int
	err := row.Scan(&pol.AccountID, &pol.PluginID, &enabled, &pol.SettingsJSON, &pol.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return models.AccountPluginPolicy{
			AccountID: accountID,
			PluginID:  pluginID,
			Enabled:   true,
		}, nil
	}
	if err != nil {
		return pol, err
	}
	pol.Enabled = enabled != 0
	return pol, nil
}

// ListPluginPolicies returns effective policies for all catalog plugins in an account.
// Plugins without an explicit policy row are included as enabled=true.
func (p *Platform) ListPluginPolicies(accountID int64) ([]models.AccountPluginPolicy, error) {
	// Fetch existing rows
	rows, err := p.DB.Query(
		`SELECT account_id, plugin_id, enabled, settings_json, updated_at
		 FROM account_plugin_policies WHERE account_id=? ORDER BY plugin_id`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	existing := map[string]models.AccountPluginPolicy{}
	for rows.Next() {
		var pol models.AccountPluginPolicy
		var enabled int
		if err := rows.Scan(&pol.AccountID, &pol.PluginID, &enabled, &pol.SettingsJSON, &pol.UpdatedAt); err != nil {
			return nil, err
		}
		pol.Enabled = enabled != 0
		existing[pol.PluginID] = pol
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Merge with catalog so every registered plugin appears
	cat := plugins.Catalog()
	out := make([]models.AccountPluginPolicy, 0, len(cat))
	for _, d := range cat {
		if pol, ok := existing[d.ID]; ok {
			out = append(out, pol)
		} else {
			out = append(out, models.AccountPluginPolicy{
				AccountID:    accountID,
				PluginID:     d.ID,
				Enabled:      true,
				SettingsJSON: "{}",
			})
		}
	}
	return out, nil
}

// SetPluginPolicy upserts a per-account plugin policy.
// Pass nil for enabled or empty settings to leave those fields unchanged.
func (p *Platform) SetPluginPolicy(accountID int64, pluginID string, enabled *bool, settings json.RawMessage) (*models.AccountPluginPolicy, error) {
	// Verify plugin exists in catalog
	if _, _, ok := plugins.Lookup(pluginID); !ok {
		return nil, errors.New("unknown catalog plugin")
	}

	now := db.NowMS()
	current, _ := p.GetPluginPolicy(accountID, pluginID)

	newEnabled := current.Enabled
	if enabled != nil {
		newEnabled = *enabled
	}
	newSettings := current.SettingsJSON
	if len(settings) > 0 && string(settings) != "null" {
		// Validate by applying to a fresh instance
		if _, fac, ok := plugins.Lookup(pluginID); ok && fac != nil {
			inst := fac()
			if err := inst.ApplyConfig(settings); err != nil {
				return nil, err
			}
		}
		newSettings = string(settings)
	}

	en := 0
	if newEnabled {
		en = 1
	}
	_, err := p.DB.Exec(
		`INSERT INTO account_plugin_policies(account_id, plugin_id, enabled, settings_json, updated_at)
		 VALUES(?,?,?,?,?)
		 ON CONFLICT(account_id, plugin_id) DO UPDATE SET enabled=excluded.enabled, settings_json=excluded.settings_json, updated_at=excluded.updated_at`,
		accountID, pluginID, en, newSettings, now)
	if err != nil {
		return nil, err
	}
	pol, err := p.GetPluginPolicy(accountID, pluginID)
	return &pol, err
}

// IsPluginEnabled returns whether a catalog plugin is enabled for the given account.
// Default (no row) is true.
func (p *Platform) IsPluginEnabled(accountID int64, pluginID string) bool {
	pol, err := p.GetPluginPolicy(accountID, pluginID)
	if err != nil {
		return true // fail-open: default enabled
	}
	return pol.Enabled
}
