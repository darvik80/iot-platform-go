package api

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"

	"github.com/go-chi/chi/v5"

	"cogitor-iot-platform/internal/auth"
	"cogitor-iot-platform/internal/services"
)

func (s *Server) requireAdmin(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c := claimsFrom(r)
		if c == nil || !auth.CanAdmin(c.ActiveRole()) {
			writeErr(w, http.StatusForbidden, "admin access required")
			return
		}
		h(w, r)
	}
}

func (s *Server) authConfig(w http.ResponseWriter, r *http.Request) {
	if s.Plat == nil {
		// Setup mode — return minimal config
		writeJSON(w, http.StatusOK, map[string]any{
			"register_enabled":     false,
			"require_email_verify": false,
			"google_enabled":       false,
			"platform_name":        "IoT Platform",
		})
		return
	}
	writeJSON(w, http.StatusOK, s.Plat.AuthPublicConfig())
}

func (s *Server) register(w http.ResponseWriter, r *http.Request) {
	var in services.RegisterInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	u, err := s.Plat.Register(in)
	if err != nil {
		// still created but email failed → 201 with warning
		if u != nil {
			writeJSON(w, http.StatusCreated, map[string]any{
				"user":    u,
				"message": "registered; check email to verify",
				"warning": err.Error(),
			})
			return
		}
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	msg := "registered"
	if s.Plat.Cfg.Register.RequireVerify {
		msg = "registered; check email to verify (EMAIL_MODE=log prints link in server logs)"
	}
	writeJSON(w, http.StatusCreated, map[string]any{"user": u, "message": msg})
}

func (s *Server) verifyEmail(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	u, err := s.Plat.VerifyEmail(token)
	if err != nil {
		// browser-friendly
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("<h1>Verification failed</h1><p>" + err.Error() + "</p>"))
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte("<h1>Email verified</h1><p>Account <b>" + u.Login + "</b> is ready. <a href=\"/#login\">Sign in</a></p>"))
}

func (s *Server) resendVerification(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email string `json:"email"`
		Login string `json:"login"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	identity := in.Email
	if identity == "" {
		identity = in.Login
	}
	_ = s.Plat.ResendVerification(identity)
	writeJSON(w, http.StatusOK, map[string]string{"message": "if the account exists, a verification email was sent"})
}

func (s *Server) checkSlugAvailable(w http.ResponseWriter, r *http.Request) {
	slug := r.URL.Query().Get("slug")
	if slug == "" {
		writeErr(w, http.StatusBadRequest, "missing slug parameter")
		return
	}
	available := s.Plat.CheckSlugAvailable(slug)
	writeJSON(w, http.StatusOK, map[string]any{"available": available, "slug": slug})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email    string `json:"email"`
		Login    string `json:"login"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	identity := in.Email
	if identity == "" {
		identity = in.Login
	}
	u, tok, err := s.Plat.Login(identity, in.Password)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"token": tok, "user": u})
}

func (s *Server) googleStart(w http.ResponseWriter, r *http.Request) {
	u, err := s.Plat.GoogleAuthURL()
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	http.Redirect(w, r, u, http.StatusFound)
}

func (s *Server) googleCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	if code == "" {
		writeErr(w, http.StatusBadRequest, "missing code")
		return
	}
	_, tok, err := s.Plat.GoogleCallback(r.Context(), code, state)
	if err != nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("<h1>Google sign-in failed</h1><p>" + err.Error() + "</p>"))
		return
	}
	// Redirect to SPA with token in hash (not query — avoid referrer leaks a bit)
	http.Redirect(w, r, "/#auth="+url.QueryEscape(tok), http.StatusFound)
}

func (s *Server) listUsers(w http.ResponseWriter, r *http.Request) {
	list, err := s.Plat.ListUsers()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) updateUser(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var in services.UpdateUserInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	u, err := s.Plat.UpdateUser(id, in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, u)
}

func (s *Server) deleteUser(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	me := userFrom(r)
	if me != nil && me.ID == id {
		writeErr(w, http.StatusBadRequest, "cannot delete yourself")
		return
	}
	if err := s.Plat.DeleteUser(id); err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) listAPITokens(w http.ResponseWriter, r *http.Request) {
	c := claimsFrom(r)
	var accID int64
	if c != nil {
		accID = c.ActiveAccountID
	}
	list, err := s.Plat.ListAPITokens(userFrom(r).ID, accID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) createAPIToken(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	c := claimsFrom(r)
	var accID int64
	if c != nil {
		accID = c.ActiveAccountID
	}
	tok, err := s.Plat.CreateAPIToken(userFrom(r).ID, accID, in.Name)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, tok)
}

func (s *Server) revokeAPIToken(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err := s.Plat.RevokeAPIToken(userFrom(r).ID, id); err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── Avatar ──────────────────────────────────────────────────────────────────

func (s *Server) getMyAvatar(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	path := s.Plat.AvatarFilePath(u.ID)
	if path == "" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=86400")
	http.ServeFile(w, r, path)
}

func (s *Server) uploadMyAvatar(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 3<<20) // 3 MB limit with overhead
	if err := r.ParseMultipartForm(3 << 20); err != nil {
		writeErr(w, http.StatusBadRequest, "file too large or invalid multipart")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "file field required")
		return
	}
	u, err := s.Plat.SetAvatarFromFile(userFrom(r).ID, file, header.Header.Get("Content-Type"), header.Filename)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, u)
}

func (s *Server) deleteMyAvatar(w http.ResponseWriter, r *http.Request) {
	if err := s.Plat.DeleteAvatar(userFrom(r).ID); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
