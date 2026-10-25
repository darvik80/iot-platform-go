package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"cogitor-iot-platform/internal/models"
)

func (s *Server) deviceGroupRoutes(r chi.Router) {
	r.Route("/device-groups", func(r chi.Router) {
		r.Get("/", s.listDeviceGroups)
		r.Post("/", s.requireWrite(s.createDeviceGroup))
		r.Get("/{id}", s.getDeviceGroup)
		r.Put("/{id}", s.requireWrite(s.updateDeviceGroup))
		r.Delete("/{id}", s.requireWrite(s.deleteDeviceGroup))
		r.Get("/{id}/devices", s.getDeviceGroupDevices)
		r.Put("/{id}/devices", s.requireWrite(s.setDeviceGroupDevices))
	})
}

func (s *Server) listDeviceGroups(w http.ResponseWriter, r *http.Request) {
	accountID := accountFrom(r.Context())
	groups, err := s.Plat.ListDeviceGroups(accountID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, groups)
}

func (s *Server) getDeviceGroup(w http.ResponseWriter, r *http.Request) {
	accountID := accountFrom(r.Context())
	groupID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || groupID <= 0 {
		writeErr(w, http.StatusBadRequest, "invalid group id")
		return
	}
	group, err := s.Plat.GetDeviceGroup(accountID, groupID)
	if err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, group)
}

func (s *Server) createDeviceGroup(w http.ResponseWriter, r *http.Request) {
	accountID := accountFrom(r.Context())
	var in models.DeviceGroupInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	group, err := s.Plat.SaveDeviceGroup(accountID, 0, in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, group)
}

func (s *Server) updateDeviceGroup(w http.ResponseWriter, r *http.Request) {
	accountID := accountFrom(r.Context())
	groupID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || groupID <= 0 {
		writeErr(w, http.StatusBadRequest, "invalid group id")
		return
	}
	var in models.DeviceGroupInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	group, err := s.Plat.SaveDeviceGroup(accountID, groupID, in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, group)
}

func (s *Server) deleteDeviceGroup(w http.ResponseWriter, r *http.Request) {
	accountID := accountFrom(r.Context())
	groupID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || groupID <= 0 {
		writeErr(w, http.StatusBadRequest, "invalid group id")
		return
	}
	if err := s.Plat.DeleteDeviceGroup(accountID, groupID); err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) getDeviceGroupDevices(w http.ResponseWriter, r *http.Request) {
	accountID := accountFrom(r.Context())
	groupID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || groupID <= 0 {
		writeErr(w, http.StatusBadRequest, "invalid group id")
		return
	}
	devices, err := s.Plat.GetDeviceGroupMembers(accountID, groupID)
	if err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, devices)
}

func (s *Server) setDeviceGroupDevices(w http.ResponseWriter, r *http.Request) {
	accountID := accountFrom(r.Context())
	groupID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || groupID <= 0 {
		writeErr(w, http.StatusBadRequest, "invalid group id")
		return
	}
	var in models.DeviceGroupMembers
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := s.Plat.SetDeviceGroupMembers(accountID, groupID, in.DeviceIDs); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
