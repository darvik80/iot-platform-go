package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
)

// createAccount handles POST /accounts
// Only platform admins can create accounts.
func (s *Server) createAccount(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Slug string `json:"slug"`
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}

	uid := userFrom(r).ID
	acc, err := s.Plat.CreateAccount(in.Slug, in.Name, uid)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	// Audit
	s.Plat.Audit(&uid, "platform_admin", "create", "account", acc.Slug, map[string]any{"id": acc.ID, "name": acc.Name})

	writeJSON(w, http.StatusCreated, acc)
}

// deleteAccount handles DELETE /accounts/{id}?force=true
// Only platform admins can delete accounts.
func (s *Server) deleteAccount(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	force := r.URL.Query().Get("force") == "true"

	if err := s.Plat.DeleteAccount(id, force); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	// Audit
	uid := userFrom(r).ID
	s.Plat.Audit(&uid, "platform_admin", "delete", "account", strconv.FormatInt(id, 10), map[string]any{"force": force})

	w.WriteHeader(http.StatusNoContent)
}
