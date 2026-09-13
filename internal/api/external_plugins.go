package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"cogitor-iot-platform/internal/services"
)

// ─── Management endpoints (admin JWT auth) ───────────────────────────────────

func (s *Server) listExternalPlugins(w http.ResponseWriter, r *http.Request) {
	list := s.Plat.ExternalPlugins.ListAll()
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) registerExternalPlugin(w http.ResponseWriter, r *http.Request) {
	var in services.RegisterExternalPluginInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	// Set account from JWT context
	in.AccountID = accountFrom(r.Context())
	slug, err := s.Plat.AccountSlugByID(in.AccountID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "account not found")
		return
	}
	in.AccountSlug = slug

	plugin, mqttPass, err := s.Plat.ExternalPlugins.Register(in)
	if err != nil {
		if s.Plat.Log != nil {
			s.Plat.Log.Warn("external plugin register failed", "id", in.ID, "err", err)
		}
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if s.Plat.Log != nil {
		s.Plat.Log.Info("external plugin register OK",
			"id", plugin.ID, "account", in.AccountSlug, "mqtt_user", plugin.MQTTUser)
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"ok":        true,
		"id":        plugin.ID,
		"mqtt_user": plugin.MQTTUser,
		"mqtt_pass": mqttPass,
		"mqtt_port": s.Plat.Cfg.MQTTPublicPort(),
		"mqtt_host": s.Plat.Cfg.BrokerHostPublic,
		"mqtt_ws":   s.Plat.Cfg.MQTTWSPublicURL(),
		"rules_bus": s.Plat.Cfg.RulesBusPrefix,
	})
}

func (s *Server) deleteExternalPlugin(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := s.Plat.ExternalPlugins.Delete(id); err != nil {
		if s.Plat.Log != nil {
			s.Plat.Log.Warn("external plugin delete failed", "id", id, "err", err)
		}
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if s.Plat.Log != nil {
		s.Plat.Log.Info("external plugin deleted via API", "id", id)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) regenerateExternalPluginMQTT(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	newPass, err := s.Plat.ExternalPlugins.RegenerateMQTTPassword(id)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	plugin, _ := s.Plat.ExternalPlugins.GetPlugin(id)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":        true,
		"id":        id,
		"mqtt_user": plugin.MQTTUser,
		"mqtt_pass": newPass,
	})
}
