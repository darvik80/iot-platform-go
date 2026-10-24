package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"cogitor-iot-platform/internal/models"
	"cogitor-iot-platform/internal/services"
)

func (s *Server) monitoringRoutes(r chi.Router) {
	r.Route("/monitoring", func(r chi.Router) {
		r.Get("/dashboards", monitorList(s, s.Plat.ListMonitoringDashboards))
		r.Post("/dashboards", s.monitorWrite(false, monitorSave(s, s.Plat.SaveMonitoringDashboard)))
		r.Get("/dashboards/{id}", func(w http.ResponseWriter, r *http.Request) {
			id, ok := monitorID(w, chi.URLParam(r, "id"))
			if !ok {
				return
			}
			out, err := s.Plat.GetMonitoringDashboard(accountFrom(r.Context()), id)
			s.monitorResult(w, r, http.StatusOK, out, err)
		})
		r.Put("/dashboards/{id}", s.monitorWrite(false, monitorSave(s, s.Plat.SaveMonitoringDashboard)))
		r.Delete("/dashboards/{id}", s.monitorWrite(false, monitorDelete(s, s.Plat.DeleteMonitoringDashboard)))
		r.Get("/metrics", func(w http.ResponseWriter, r *http.Request) {
			id, ok := monitorID(w, r.URL.Query().Get("device_id"))
			if !ok {
				return
			}
			out, err := s.Plat.MonitoringMetrics(accountFrom(r.Context()), id)
			s.monitorResult(w, r, http.StatusOK, out, err)
		})
		r.Post("/query", func(w http.ResponseWriter, r *http.Request) {
			var in models.MonitoringQuery
			if !monitorDecode(w, r, &in) {
				return
			}
			out, err := s.Plat.QueryMonitoring(accountFrom(r.Context()), in)
			s.monitorResult(w, r, http.StatusOK, out, err)
		})
		r.Get("/rules", monitorList(s, s.Plat.ListMonitorRules))
		r.Post("/rules", s.monitorWrite(true, monitorSave(s, s.Plat.SaveMonitorRule)))
		r.Put("/rules/{id}", s.monitorWrite(true, monitorSave(s, s.Plat.SaveMonitorRule)))
		r.Delete("/rules/{id}", s.monitorWrite(true, monitorDelete(s, s.Plat.DeleteMonitorRule)))
		r.Get("/channels", s.monitorWrite(true, monitorList(s, s.Plat.ListMonitorChannels)))
		r.Post("/channels", s.monitorWrite(true, monitorSave(s, s.Plat.SaveMonitorChannel)))
		r.Put("/channels/{id}", s.monitorWrite(true, monitorSave(s, s.Plat.SaveMonitorChannel)))
		r.Delete("/channels/{id}", s.monitorWrite(true, monitorDelete(s, s.Plat.DeleteMonitorChannel)))
		r.Post("/channels/{id}/test", s.monitorWrite(true, func(w http.ResponseWriter, r *http.Request) {
			id, ok := monitorID(w, chi.URLParam(r, "id"))
			if !ok {
				return
			}
			err := s.Plat.TestMonitorChannel(r.Context(), accountFrom(r.Context()), id)
			s.monitorResult(w, r, http.StatusNoContent, nil, err)
		}))
		r.Get("/events", monitorHistory(s, s.Plat.ListMonitorEvents))
		r.Get("/deliveries", monitorHistory(s, s.Plat.ListMonitorDeliveries))
	})
}

func (s *Server) monitorWrite(adminOnly bool, h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var role string
		err := s.Plat.DB.QueryRow(`SELECT role FROM account_memberships WHERE account_id=? AND user_id=? AND status='active'`, accountFrom(r.Context()), userFrom(r).ID).Scan(&role)
		if err != nil || (role != "admin" && (adminOnly || role != "operator")) {
			writeErr(w, http.StatusForbidden, "insufficient monitoring permissions")
			return
		}
		h(w, r)
	}
}

func monitorID(w http.ResponseWriter, raw string) (int64, bool) {
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		writeErr(w, http.StatusBadRequest, "invalid resource id")
		return 0, false
	}
	return id, true
}

func monitorDecode(w http.ResponseWriter, r *http.Request, out any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 128<<10)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	err := decoder.Decode(out)
	if err == nil {
		var extra any
		if decoder.Decode(&extra) != io.EOF {
			err = errors.New("trailing json")
		}
	}
	if err != nil {
		var tooLarge *http.MaxBytesError
		status := http.StatusBadRequest
		if errors.As(err, &tooLarge) {
			status = http.StatusRequestEntityTooLarge
		}
		writeErr(w, status, "invalid monitoring json")
		return false
	}
	return true
}

func (s *Server) monitorResult(w http.ResponseWriter, r *http.Request, status int, out any, err error) {
	if err != nil {
		switch {
		case errors.Is(err, services.ErrMonitorInvalid):
			writeErr(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, services.ErrMonitorNotFound):
			writeErr(w, http.StatusNotFound, "monitoring resource not found")
		case errors.Is(err, services.ErrMonitorConflict):
			writeErr(w, http.StatusConflict, err.Error())
		default:
			s.Plat.Log.ErrorContext(r.Context(), "monitoring request failed", "path", r.URL.Path, "err", err)
			writeErr(w, http.StatusInternalServerError, "monitoring request failed")
		}
		return
	}
	if status == http.StatusNoContent {
		w.WriteHeader(status)
		return
	}
	writeJSON(w, status, out)
}

func monitorList[T any](s *Server, list func(int64) ([]T, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		out, err := list(accountFrom(r.Context()))
		s.monitorResult(w, r, http.StatusOK, out, err)
	}
}

func monitorHistory[T any](s *Server, list func(int64, int) ([]T, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit := 100
		if raw := r.URL.Query().Get("limit"); raw != "" {
			var err error
			limit, err = strconv.Atoi(raw)
			if err != nil || limit < 1 || limit > 500 {
				writeErr(w, http.StatusBadRequest, "limit must be between 1 and 500")
				return
			}
		}
		out, err := list(accountFrom(r.Context()), limit)
		s.monitorResult(w, r, http.StatusOK, out, err)
	}
}

func monitorSave[I, O any](s *Server, save func(int64, int64, I) (*O, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var id int64
		status := http.StatusCreated
		if r.Method == http.MethodPut {
			var ok bool
			id, ok = monitorID(w, chi.URLParam(r, "id"))
			if !ok {
				return
			}
			status = http.StatusOK
		}
		var in I
		if !monitorDecode(w, r, &in) {
			return
		}
		out, err := save(accountFrom(r.Context()), id, in)
		s.monitorResult(w, r, status, out, err)
	}
}

func monitorDelete(s *Server, remove func(int64, int64) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := monitorID(w, chi.URLParam(r, "id"))
		if !ok {
			return
		}
		err := remove(accountFrom(r.Context()), id)
		s.monitorResult(w, r, http.StatusNoContent, nil, err)
	}
}
