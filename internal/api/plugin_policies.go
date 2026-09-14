package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"cogitor-iot-platform/internal/plugins"
)

// policyView is the API response shape for a single plugin policy entry.
type policyView struct {
	ID            string          `json:"id"`
	Name          string          `json:"name"`
	Description   string          `json:"description"`
	Enabled       bool            `json:"enabled"`
	SettingsJSON  string          `json:"settings_json"`
	Form          json.RawMessage `json:"form,omitempty"`
	DefaultConfig json.RawMessage `json:"default_config,omitempty"`
	UpdatedAt     int64           `json:"updated_at,omitempty"`
}

// listPluginPolicies returns all catalog plugins with their effective policy for the active account.
func (s *Server) listPluginPolicies(w http.ResponseWriter, r *http.Request) {
	accountID := accountFrom(r.Context())
	policies, err := s.Plat.ListPluginPolicies(accountID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Build lookup from policy rows
	polMap := make(map[string]int) // pluginID -> index in policies slice
	for i, p := range policies {
		polMap[p.PluginID] = i
	}

	// Merge with catalog descriptors
	cat := plugins.Catalog()
	out := make([]policyView, 0, len(cat))
	for _, d := range cat {
		pv := policyView{
			ID:            d.ID,
			Name:          d.Name,
			Description:   d.Description,
			Form:          plugins.FormJSON(d.Form),
			DefaultConfig: d.DefaultConfig,
		}
		if idx, ok := polMap[d.ID]; ok {
			p := policies[idx]
			pv.Enabled = p.Enabled
			pv.SettingsJSON = p.SettingsJSON
			pv.UpdatedAt = p.UpdatedAt
		} else {
			pv.Enabled = true
			pv.SettingsJSON = "{}"
		}
		out = append(out, pv)
	}
	writeJSON(w, http.StatusOK, out)
}

// updatePluginPolicy patches the policy for a single catalog plugin.
func (s *Server) updatePluginPolicy(w http.ResponseWriter, r *http.Request) {
	accountID := accountFrom(r.Context())
	pluginID := chi.URLParam(r, "pluginId")

	var in struct {
		Enabled  *bool           `json:"enabled"`
		Settings json.RawMessage `json:"settings"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}

	pol, err := s.Plat.SetPluginPolicy(accountID, pluginID, in.Enabled, in.Settings)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	// Apply on the fly: rebuild runners for this account
	if s.Plat.Plugins != nil {
		s.Plat.Plugins.RebuildAccountRunners(accountID)
	}

	uid := userFrom(r).ID
	s.Plat.Audit(&uid, "user", "policy.update", "plugin_policy", pluginID, map[string]any{
		"account_id": accountID,
		"enabled":    pol.Enabled,
	})

	writeJSON(w, http.StatusOK, pol)
}
