package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"cogitor-iot-platform/internal/services"
)

func (s *Server) listPluginCatalog(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.Plat.ListPluginCatalog())
}

// scopedDevicePK returns the {id} device if it belongs to the active account.
func (s *Server) scopedDevicePK(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if _, err := s.Plat.GetDeviceScoped(accountFrom(r.Context()), id); err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return 0, false
	}
	return id, true
}

// scopedBindingID returns {bindingId} if it is attached to the scoped {id} device.
func (s *Server) scopedBindingID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	devicePK, ok := s.scopedDevicePK(w, r)
	if !ok {
		return 0, false
	}
	bindingID, _ := strconv.ParseInt(chi.URLParam(r, "bindingId"), 10, 64)
	b, err := s.Plat.GetPluginBinding(bindingID)
	if err != nil || b.DevicePK != devicePK {
		writeErr(w, http.StatusNotFound, "plugin binding not found")
		return 0, false
	}
	return bindingID, true
}

func (s *Server) listDevicePlugins(w http.ResponseWriter, r *http.Request) {
	id, ok := s.scopedDevicePK(w, r)
	if !ok {
		return
	}
	list, err := s.Plat.ListDevicePlugins(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) bindDevicePlugin(w http.ResponseWriter, r *http.Request) {
	id, ok := s.scopedDevicePK(w, r)
	if !ok {
		return
	}
	var in services.PluginBindingInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	b, err := s.Plat.BindDevicePlugin(id, in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, b)
}

func (s *Server) updateDevicePlugin(w http.ResponseWriter, r *http.Request) {
	bindingID, ok := s.scopedBindingID(w, r)
	if !ok {
		return
	}
	var in services.PluginBindingInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	b, err := s.Plat.UpdatePluginBinding(bindingID, in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, b)
}

func (s *Server) deleteDevicePlugin(w http.ResponseWriter, r *http.Request) {
	bindingID, ok := s.scopedBindingID(w, r)
	if !ok {
		return
	}
	if err := s.Plat.DeletePluginBinding(bindingID); err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) applyDevicePlugin(w http.ResponseWriter, r *http.Request) {
	bindingID, ok := s.scopedBindingID(w, r)
	if !ok {
		return
	}
	if s.Plat.Plugins == nil {
		writeErr(w, http.StatusBadRequest, "plugins not running")
		return
	}
	if err := s.Plat.Plugins.Apply(bindingID); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	b, err := s.Plat.GetPluginBinding(bindingID)
	if err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, b)
}
