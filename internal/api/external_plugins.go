package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"crearts-iot-platform/internal/auth"
	"crearts-iot-platform/internal/services"
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
	plugin, token, mqttPass, err := s.Plat.ExternalPlugins.Register(in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"ok":        true,
		"id":        plugin.ID,
		"token":     token,
		"mqtt_user": plugin.MQTTUser,
		"mqtt_pass": mqttPass,
		"mqtt_port": s.Plat.Cfg.MQTTPublicPort(),
		"mqtt_host": s.Plat.Cfg.BrokerHostPublic,
		"rules_bus": s.Plat.Cfg.RulesBusPrefix,
	})
}

func (s *Server) deleteExternalPlugin(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := s.Plat.ExternalPlugins.Delete(id); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
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

// JSON-RPC endpoint for external plugins (heartbeat, poll, invoke_rpc, etc.)
func (s *Server) externalPluginRPC(w http.ResponseWriter, r *http.Request) {
	// Extract plugin token from Authorization header
	tok := auth.Bearer(r)
	if tok == "" {
		writeErr(w, http.StatusUnauthorized, "missing token")
		return
	}

	var req struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      any             `json:"id"`
		Method  string          `json:"method"`
		Params  json.RawMessage `json:"params"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONRPCError(w, req.ID, -32700, "parse error")
		return
	}

	// Route methods
	switch req.Method {
	case "platform.heartbeat":
		s.handlePluginHeartbeat(w, req.ID, tok, req.Params)
	case "platform.poll_events":
		s.handlePluginPollEvents(w, req.ID, tok, req.Params)
	case "platform.ack_events":
		s.handlePluginAckEvents(w, req.ID, tok, req.Params)
	case "platform.invoke_rpc":
		s.handlePluginInvokeRPC(w, req.ID, tok, req.Params)
	default:
		writeJSONRPCError(w, req.ID, -32601, "method not found")
	}
}

func (s *Server) handlePluginHeartbeat(w http.ResponseWriter, id any, token string, params json.RawMessage) {
	// Find plugin by token
	pluginID, ok := s.Plat.ExternalPlugins.GetPluginIDByToken(token)
	if !ok {
		writeJSONRPCError(w, id, -32602, "invalid token")
		return
	}

	var in struct {
		PluginID string                  `json:"plugin_id"`
		Status   string                  `json:"status"`
		Stats    services.HeartbeatStats `json:"stats"`
	}
	if err := json.Unmarshal(params, &in); err != nil {
		writeJSONRPCError(w, id, -32602, "invalid params")
		return
	}

	if err := s.Plat.ExternalPlugins.Heartbeat(pluginID, in.Stats); err != nil {
		writeJSONRPCError(w, id, -32603, err.Error())
		return
	}

	writeJSONRPCResult(w, id, map[string]any{"ok": true})
}

func (s *Server) handlePluginPollEvents(w http.ResponseWriter, id any, token string, params json.RawMessage) {
	pluginID, ok := s.Plat.ExternalPlugins.GetPluginIDByToken(token)
	if !ok {
		writeJSONRPCError(w, id, -32602, "invalid token")
		return
	}

	var in struct {
		Limit int `json:"limit"`
	}
	_ = json.Unmarshal(params, &in)
	if in.Limit <= 0 {
		in.Limit = 100
	}

	events, err := s.Plat.ExternalPlugins.PollEvents(pluginID, in.Limit)
	if err != nil {
		writeJSONRPCError(w, id, -32603, err.Error())
		return
	}

	writeJSONRPCResult(w, id, map[string]any{"events": events})
}

func (s *Server) handlePluginAckEvents(w http.ResponseWriter, id any, token string, params json.RawMessage) {
	pluginID, ok := s.Plat.ExternalPlugins.GetPluginIDByToken(token)
	if !ok {
		writeJSONRPCError(w, id, -32602, "invalid token")
		return
	}

	var in struct {
		EventIDs []int64 `json:"event_ids"`
	}
	if err := json.Unmarshal(params, &in); err != nil {
		writeJSONRPCError(w, id, -32602, "invalid params")
		return
	}

	if err := s.Plat.ExternalPlugins.AckEvents(pluginID, in.EventIDs); err != nil {
		writeJSONRPCError(w, id, -32603, err.Error())
		return
	}

	writeJSONRPCResult(w, id, map[string]any{"ok": true})
}

func (s *Server) handlePluginInvokeRPC(w http.ResponseWriter, id any, token string, params json.RawMessage) {
	pluginID, ok := s.Plat.ExternalPlugins.GetPluginIDByToken(token)
	if !ok {
		writeJSONRPCError(w, id, -32602, "invalid token")
		return
	}

	var in struct {
		BindingID int64          `json:"binding_id"`
		Method    string         `json:"method"`
		Params    map[string]any `json:"params"`
		TimeoutMS int            `json:"timeout_ms"`
	}
	if err := json.Unmarshal(params, &in); err != nil {
		writeJSONRPCError(w, id, -32602, "invalid params")
		return
	}

	if in.TimeoutMS <= 0 {
		in.TimeoutMS = 4000
	}

	err := s.Plat.InvokeRPCForPlugin(pluginID, in.BindingID, in.Method, in.Params, in.TimeoutMS)
	if err != nil {
		writeJSONRPCError(w, id, -32603, err.Error())
		return
	}

	writeJSONRPCResult(w, id, map[string]any{"sent": true})
}

// JSON-RPC helpers

func writeJSONRPCResult(w http.ResponseWriter, id any, result any) {
	writeJSON(w, http.StatusOK, map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"result":  result,
	})
}

func writeJSONRPCError(w http.ResponseWriter, id any, code int, message string) {
	writeJSON(w, http.StatusOK, map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"error": map[string]any{
			"code":    code,
			"message": message,
		},
	})
}
