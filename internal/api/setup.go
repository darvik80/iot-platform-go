package api

import (
	"encoding/json"
	"net/http"

	"cogitor-iot-platform/internal/services"
)

// SetupHandler handles setup wizard API endpoints.
type SetupHandler struct {
	Setup *services.SetupService
}

// GetState returns the current setup state.
func (h *SetupHandler) GetState(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.Setup.View())
}

// UpdateStepRequest is the request body for updating a setup step.
type UpdateStepRequest struct {
	Step int                    `json:"step"`
	Data map[string]interface{} `json:"data"`
}

// UpdateStep updates the state for a specific setup step.
func (h *SetupHandler) UpdateStep(w http.ResponseWriter, r *http.Request) {
	var req UpdateStepRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if req.Step < 1 || req.Step > 8 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid step number"})
		return
	}

	if err := h.Setup.UpdateStep(req.Step, req.Data); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success":      true,
		"current_step": h.Setup.View().CurrentStep,
	})
}

// Apply executes the setup wizard and creates the initial database.
func (h *SetupHandler) Apply(w http.ResponseWriter, r *http.Request) {
	if err := h.Setup.Apply(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success":   true,
		"completed": true,
		"message":   "Setup completed successfully. Please restart the application.",
	})
}

// GenerateTLSRequest is the request body for generating MQTT TLS certificates.
type GenerateTLSRequest struct {
	Host string `json:"host"`
	Dir  string `json:"dir"`
}

// GenerateTLS generates self-signed MQTT TLS certificates.
func (h *SetupHandler) GenerateTLS(w http.ResponseWriter, r *http.Request) {
	var req GenerateTLSRequest
	_ = json.NewDecoder(r.Body).Decode(&req)

	result, err := h.Setup.GenerateMQTTTLS(req.Host, req.Dir)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success":   true,
		"cert_path": result.CertPath,
		"key_path":  result.KeyPath,
		"ca_path":   result.CAPath,
	})
}
