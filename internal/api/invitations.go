package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"cogitor-iot-platform/internal/models"
	"cogitor-iot-platform/internal/services"
)

// ─── Invitation endpoints ─────────────────────────────────────────────────────

func (s *Server) getInvitationInfo(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	info, err := s.Plat.GetInvitation(token)
	if err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, info)
}

func (s *Server) acceptInvitation(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	u := userFrom(r)
	if u == nil {
		writeErr(w, http.StatusUnauthorized, "authentication required")
		return
	}
	m, err := s.Plat.AcceptInvitation(token, u.ID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, m)
}

func (s *Server) createInvitation(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email          string `json:"email"`
		Role           string `json:"role"`
		DeliveryMethod string `json:"delivery_method"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	accID := accountFrom(r.Context())
	u := userFrom(r)
	inv, inviteURL, err := s.Plat.CreateInvitation(accID, u.ID, in.Email, in.Role, in.DeliveryMethod)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"invitation": inv,
		"invite_url": inviteURL,
	})
}

func (s *Server) listInvitations(w http.ResponseWriter, r *http.Request) {
	accID := accountFrom(r.Context())
	list, err := s.Plat.ListInvitations(accID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) resendInvitation(w http.ResponseWriter, r *http.Request) {
	accID := accountFrom(r.Context())
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	inviteURL, err := s.Plat.ResendInvitation(accID, id)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"invite_url": inviteURL})
}

func (s *Server) revokeInvitation(w http.ResponseWriter, r *http.Request) {
	accID := accountFrom(r.Context())
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err := s.Plat.RevokeInvitation(accID, id); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ─── Member management endpoints ──────────────────────────────────────────────

func (s *Server) createMember(w http.ResponseWriter, r *http.Request) {
	var in services.CreateMemberInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	accID := accountFrom(r.Context())
	u := userFrom(r)
	member, err := s.Plat.CreateMember(accID, u.ID, in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, member)
}

func (s *Server) listMembers(w http.ResponseWriter, r *http.Request) {
	accID := accountFrom(r.Context())
	list, err := s.Plat.ListAccountMembers(accID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) updateMemberRole(w http.ResponseWriter, r *http.Request) {
	accID := accountFrom(r.Context())
	userID, _ := strconv.ParseInt(chi.URLParam(r, "userId"), 10, 64)
	var in struct {
		Role string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := s.Plat.UpdateMemberRole(accID, userID, models.Role(in.Role)); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) removeMember(w http.ResponseWriter, r *http.Request) {
	accID := accountFrom(r.Context())
	userID, _ := strconv.ParseInt(chi.URLParam(r, "userId"), 10, 64)
	if err := s.Plat.RemoveMember(accID, userID); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
