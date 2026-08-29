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

func (s *Server) listDevicePlugins(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	list, err := s.Plat.ListDevicePlugins(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) bindDevicePlugin(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
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
	bindingID, _ := strconv.ParseInt(chi.URLParam(r, "bindingId"), 10, 64)
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
	bindingID, _ := strconv.ParseInt(chi.URLParam(r, "bindingId"), 10, 64)
	if err := s.Plat.DeletePluginBinding(bindingID); err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) applyDevicePlugin(w http.ResponseWriter, r *http.Request) {
	bindingID, _ := strconv.ParseInt(chi.URLParam(r, "bindingId"), 10, 64)
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
