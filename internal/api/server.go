package api

import (
	"context"
	"encoding/json"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"cogitor-iot-platform/internal/auth"
	"cogitor-iot-platform/internal/mcpbridge"
	"cogitor-iot-platform/internal/models"
	"cogitor-iot-platform/internal/plugins"
	"cogitor-iot-platform/internal/protocol"
	"cogitor-iot-platform/internal/services"
	"cogitor-iot-platform/internal/ws"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Server struct {
	Plat    *services.Platform
	Hub     *ws.Hub
	SpaFS   fs.FS                  // optional embedded SPA (dist contents)
	Setup   *services.SetupService // setup wizard service (nil if setup not needed)
	Version string                 // build version (injected via ldflags)
	Commit  string                 // git commit hash (injected via ldflags)

	limits *authLimits
}

func (s *Server) Router() http.Handler {
	if s.limits == nil {
		s.limits = newAuthLimits()
	}
	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.RealIP, middleware.Logger, middleware.Recoverer)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders: []string{
			"Accept", "Authorization", "Content-Type",
			"Mcp-Session-Id", "Last-Event-ID", "MCP-Protocol-Version",
		},
		AllowCredentials: true,
	}))

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	r.Get("/ota/fw/{token}", s.downloadFirmware)
	r.Head("/ota/fw/{token}", s.downloadFirmware)

	r.Get("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		if s.Plat == nil {
			// Setup mode
			writeJSON(w, http.StatusOK, map[string]any{
				"db":     false,
				"mqtt":   false,
				"status": "setup",
			})
			return
		}
		mqttOK := s.Plat.Pub != nil && s.Plat.Pub.Connected()
		status := http.StatusOK
		if !mqttOK {
			status = http.StatusServiceUnavailable
		}
		st := s.Plat.MQTTStatus()
		writeJSON(w, status, map[string]any{
			"db":           true,
			"mqtt":         mqttOK,
			"mqtt_addr":    s.Plat.Cfg.MQTT.Addr,
			"mqtt_clients": st.ClientCount,
		})
	})

	// Prometheus metrics endpoint (public, no auth)
	r.Get("/metrics", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		_, _ = w.Write([]byte(plugins.RenderMetrics()))
	})

	// Public: CA certificate for MQTT TLS clients (no auth required — needed before connect)
	r.Get("/api/v1/mqtt/ca.pem", s.mqttCACert)

	// Setup wizard endpoints (public, no auth) — only in setup mode.
	// Once the platform runs, only a static "completed" state is exposed.
	if s.Setup != nil && s.Plat == nil {
		setupHandler := &SetupHandler{Setup: s.Setup}
		r.Route("/api/v1/setup", func(r chi.Router) {
			r.Get("/state", setupHandler.GetState)
			r.Post("/step", setupHandler.UpdateStep)
			r.Post("/apply", setupHandler.Apply)
			r.Post("/generate-tls", setupHandler.GenerateTLS)
		})
	} else {
		r.Get("/api/v1/setup/state", func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, http.StatusOK, map[string]bool{"completed": true})
		})
	}

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/auth/config", s.authConfig)
		r.Post("/auth/login", s.limitByIP(s.login))
		r.Post("/auth/register", s.limitByIP(s.register))
		r.Get("/auth/verify-email", s.verifyEmail)
		r.Post("/auth/resend-verification", s.limitByIP(s.resendVerification))
		r.Get("/auth/check-slug", s.limitByIP(s.checkSlugAvailable))
		r.Get("/auth/google", s.googleStart)
		r.Get("/auth/google/callback", s.googleCallback)

		// Public invitation endpoints (no auth required)
		r.Get("/invitations/{token}", s.getInvitationInfo)
		r.Post("/invitations/{token}/register", s.limitByIP(s.registerViaInvitation))

		r.Group(func(r chi.Router) {
			r.Use(s.authenticate)

			// Account-less endpoints (no requireAccount)
			r.Get("/me", s.me)
			r.Get("/me/accounts", s.listMyAccounts)
			r.Post("/auth/switch-account", s.switchAccount)
			r.Get("/me/tokens", s.listAPITokens)
			r.Post("/me/tokens", s.createAPIToken)
			r.Delete("/me/tokens/{id}", s.revokeAPIToken)

			// Avatar (per-user, not account-scoped)
			r.Get("/me/avatar", s.getMyAvatar)
			r.Post("/me/avatar", s.uploadMyAvatar)
			r.Delete("/me/avatar", s.deleteMyAvatar)

			// Accept invitation (auth required, no account required)
			r.Post("/invitations/{token}/accept", s.acceptInvitation)

			// Account-scoped endpoints
			r.Group(func(r chi.Router) {
				r.Use(s.requireAccount)

				r.Get("/overview", s.overview)
				r.Get("/mqtt", s.mqttStatus)

				r.Get("/users", s.requirePlatformAdmin(s.listUsers))
				r.Patch("/users/{id}", s.requireAdmin(s.updateUser))
				r.Delete("/users/{id}", s.requirePlatformAdmin(s.deleteUser))

				r.Get("/products", s.listProducts)
				r.Post("/products", s.requireWrite(s.createProduct))
				r.Get("/products/{id}", s.getProduct)
				r.Put("/products/{id}", s.requireWrite(s.updateProduct))
				r.Delete("/products/{id}", s.requireWrite(s.deleteProduct))
				r.Post("/products/{id}/esp-defaults", s.requireWrite(s.applyESPDefaults))

				r.Get("/devices", s.listDevices)
				r.Post("/devices", s.requireWrite(s.createDevice))
				r.Get("/devices/{id}", s.getDevice)
				r.Put("/devices/{id}", s.requireWrite(s.updateDevice))
				r.Delete("/devices/{id}", s.requireWrite(s.deleteDevice))
				r.Post("/devices/{id}/token/rotate", s.requireWrite(s.rotateToken))
				r.Post("/devices/{id}/enable", s.requireWrite(s.enableDevice))
				r.Post("/devices/{id}/disable", s.requireWrite(s.disableDevice))

				r.Get("/devices/{id}/telemetry", s.deviceTelemetry)
				r.Get("/devices/{id}/telemetry/latest", s.deviceTelemetryLatest)
				r.Get("/devices/{id}/attributes", s.deviceAttributes)
				r.Patch("/devices/{id}/attributes/desired", s.requireWrite(s.patchDesired))
				r.Post("/devices/{id}/rpc", s.requireWrite(s.invokeRPC))
				r.Get("/devices/{id}/rpc", s.listRPC)
				r.Post("/devices/{id}/ota", s.requireWrite(s.startOTA))
				r.Post("/devices/{id}/ota/cancel", s.requireWrite(s.cancelOTA))
				r.Get("/devices/{id}/dashboard", s.deviceDashboard)

				r.Get("/plugins", s.listPluginCatalog)
				r.Get("/devices/{id}/plugins", s.listDevicePlugins)
				r.Post("/devices/{id}/plugins", s.requireWrite(s.bindDevicePlugin))
				r.Put("/devices/{id}/plugins/{bindingId}", s.requireWrite(s.updateDevicePlugin))
				r.Delete("/devices/{id}/plugins/{bindingId}", s.requireWrite(s.deleteDevicePlugin))
				r.Post("/devices/{id}/plugins/{bindingId}/apply", s.requireWrite(s.applyDevicePlugin))
				r.Post("/ota/firmwares/{id}/notify", s.requireWrite(s.notifyFirmware))

				r.Get("/events", s.listEvents)
				r.Get("/logs", s.listLogs)
				r.Get("/ota/firmwares", s.listFirmware)
				r.Post("/ota/firmwares", s.requireWrite(s.registerFirmware))
				r.Post("/ota/firmwares/upload", s.requireWrite(s.uploadFirmware))
				r.Delete("/ota/firmwares/{id}", s.requireWrite(s.deleteFirmware))
				r.Get("/ota/jobs", s.listOTAJobs)

				r.Get("/settings", s.getSettings)

				// External plugins management (admin only)
				r.Get("/plugins/external", s.listExternalPlugins)
				r.Post("/plugins/external", s.requireAdmin(s.registerExternalPlugin))
				r.Delete("/plugins/external/{id}", s.requireAdmin(s.deleteExternalPlugin))
				r.Post("/plugins/external/{id}/regenerate-mqtt", s.requireAdmin(s.regenerateExternalPluginMQTT))

				// Plugin policies (admin only — catalog plugin management)
				r.Get("/plugins/policies", s.requireAdmin(s.listPluginPolicies))
				r.Patch("/plugins/policies/{pluginId}", s.requireAdmin(s.updatePluginPolicy))

				// Invitations and member management (admin only)
				r.Post("/invitations", s.requireAdmin(s.createInvitation))
				r.Get("/invitations", s.requireAdmin(s.listInvitations))
				r.Post("/invitations/{id}/resend", s.requireAdmin(s.resendInvitation))
				r.Delete("/invitations/{id}", s.requireAdmin(s.revokeInvitation))
				r.Post("/members", s.requireAdmin(s.createMember))
				r.Get("/members", s.requireAdmin(s.listMembers))
				r.Patch("/members/{userId}", s.requireAdmin(s.updateMemberRole))
				r.Delete("/members/{userId}", s.requireAdmin(s.removeMember))

				r.Get("/audit-log", s.requireAdmin(s.listAuditLog))

				r.Get("/ws", func(w http.ResponseWriter, r *http.Request) {
					u := userFrom(r)
					s.Hub.ServeWS(w, r, accountFrom(r.Context()), u != nil && s.Plat.IsPlatformAdmin(u.ID))
				})
			})
		})
	})

	// Embedded Streamable HTTP MCP (same process as platform — not a separate deploy).
	mcpHandler := mcp.NewStreamableHTTPHandler(func(req *http.Request) *mcp.Server {
		c := claimsFrom(req)
		role := models.Role("")
		var accID int64
		var slug string
		if c != nil {
			role = c.ActiveRole()
			accID = c.ActiveAccountID
			for _, a := range c.Accounts {
				if a.AccountID == accID {
					slug = a.AccountSlug
				}
			}
		}
		return mcpbridge.NewServer(mcpbridge.NewLocal(s.Plat, role, accID, slug))
	}, &mcp.StreamableHTTPOptions{
		Stateless: true,
		// Behind a reverse proxy requests arrive from localhost with the public Host;
		// the endpoint requires a token, which a DNS-rebinding page cannot supply.
		DisableLocalhostProtection: true,
	})
	r.Group(func(r chi.Router) {
		r.Use(s.authenticate)
		r.Use(s.requireAccount)
		r.Handle("/mcp", mcpHandler)
		r.Handle("/mcp/*", mcpHandler)
	})

	if s.SpaFS != nil {
		r.Handle("/*", spaHandler(s.SpaFS))
	}
	return r
}

type ctxKey int

const (
	userKey    ctxKey = 1
	claimsKey  ctxKey = 2
	accountKey ctxKey = 3
)

func withUser(ctx context.Context, u models.User) context.Context {
	return context.WithValue(ctx, userKey, u)
}

func userFrom(r *http.Request) *models.User {
	u, ok := r.Context().Value(userKey).(models.User)
	if !ok {
		return nil
	}
	return &u
}

func withClaims(ctx context.Context, c *auth.Claims) context.Context {
	return context.WithValue(ctx, claimsKey, c)
}

func claimsFrom(r *http.Request) *auth.Claims {
	c, _ := r.Context().Value(claimsKey).(*auth.Claims)
	return c
}

func withAccount(ctx context.Context, accountID int64) context.Context {
	return context.WithValue(ctx, accountKey, accountID)
}

func accountFrom(ctx context.Context) int64 {
	id, _ := ctx.Value(accountKey).(int64)
	return id
}

func (s *Server) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.Plat == nil {
			// Setup mode — no authenticated endpoints
			writeErr(w, http.StatusServiceUnavailable, "platform not initialized")
			return
		}
		tok := auth.Bearer(r)
		if tok == "" {
			tok = r.URL.Query().Get("token")
		}
		if tok == "" {
			writeErr(w, http.StatusUnauthorized, "missing token")
			return
		}
		claims, err := auth.ParseJWT(s.Plat.Cfg.Auth.JWTSecret, tok)
		if err == nil {
			u, err := s.Plat.GetUser(claims.UserID)
			if err != nil || u.Disabled {
				writeErr(w, http.StatusUnauthorized, "invalid token")
				return
			}
			ctx := withUser(r.Context(), *u)
			ctx = withClaims(ctx, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}
		// API token fallback — build synthetic claims
		u, tokenAccID, err := s.Plat.UserByAPIToken(tok)
		if err != nil || u.Disabled {
			writeErr(w, http.StatusUnauthorized, "invalid token")
			return
		}
		refs, activeID, _ := s.Plat.LoadAccountRefs(u.ID)
		// Use the token's account_id if set, otherwise fall back to first membership
		if tokenAccID > 0 {
			activeID = tokenAccID
		}
		synthetic := &auth.Claims{
			UserID:          u.ID,
			Email:           u.Email,
			Login:           u.Login,
			Accounts:        refs,
			ActiveAccountID: activeID,
		}
		ctx := withUser(r.Context(), *u)
		ctx = withClaims(ctx, synthetic)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// requireAccount ensures an active account is selected and membership is valid.
func (s *Server) requireAccount(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := claimsFrom(r)
		if c == nil || c.ActiveAccountID == 0 {
			writeErr(w, http.StatusForbidden, "select an account")
			return
		}
		// Verify membership is still active
		var status string
		err := s.Plat.DB.QueryRow(`SELECT status FROM account_memberships WHERE user_id=? AND account_id=?`,
			c.UserID, c.ActiveAccountID).Scan(&status)
		if err != nil || status != "active" {
			writeErr(w, http.StatusForbidden, "membership not active")
			return
		}
		// Check account not suspended
		var accStatus string
		err = s.Plat.DB.QueryRow(`SELECT status FROM accounts WHERE id=?`, c.ActiveAccountID).Scan(&accStatus)
		if err != nil || accStatus == "suspended" {
			writeErr(w, http.StatusForbidden, "account suspended")
			return
		}
		next.ServeHTTP(w, r.WithContext(withAccount(r.Context(), c.ActiveAccountID)))
	})
}

func (s *Server) requireWrite(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c := claimsFrom(r)
		if c == nil || !auth.CanWrite(c.ActiveRole()) {
			writeErr(w, http.StatusForbidden, "write access required")
			return
		}
		h(w, r)
	}
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	c := claimsFrom(r)
	memberships, _ := s.Plat.ListUserMemberships(u.ID)
	writeJSON(w, http.StatusOK, map[string]any{
		"user":              u,
		"memberships":       memberships,
		"active_account_id": c.ActiveAccountID,
	})
}

func (s *Server) listMyAccounts(w http.ResponseWriter, r *http.Request) {
	list, err := s.Plat.ListUserMemberships(userFrom(r).ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) switchAccount(w http.ResponseWriter, r *http.Request) {
	var in struct {
		AccountID int64 `json:"account_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	tok, err := s.Plat.SwitchAccount(userFrom(r).ID, in.AccountID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"token": tok})
}

func (s *Server) overview(w http.ResponseWriter, r *http.Request) {
	o, err := s.Plat.Overview(accountFrom(r.Context()))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, o)
}

func (s *Server) mqttStatus(w http.ResponseWriter, r *http.Request) {
	st := s.Plat.MQTTStatus()
	if u := userFrom(r); u != nil && s.Plat.IsPlatformAdmin(u.ID) {
		writeJSON(w, http.StatusOK, st)
		return
	}
	acc := accountFrom(r.Context())
	owned := map[string]bool{}
	mine := func(username string) bool {
		v, ok := owned[username]
		if !ok {
			v = acc != 0 && s.Plat.AccountIDForMQTTUser(username) == acc
			owned[username] = v
		}
		return v
	}
	clients := st.Clients[:0]
	for _, c := range st.Clients {
		if mine(c.Username) {
			clients = append(clients, c)
		}
	}
	st.Clients = clients
	st.ClientCount = len(clients)
	events := st.Events[:0]
	for _, e := range st.Events {
		if mine(e.Username) {
			events = append(events, e)
		}
	}
	st.Events = events
	writeJSON(w, http.StatusOK, st)
}

// mqttCACert serves the MQTT TLS CA certificate (public, no auth).
func (s *Server) mqttCACert(w http.ResponseWriter, r *http.Request) {
	if s.Plat == nil {
		// Setup mode
		writeErr(w, http.StatusServiceUnavailable, "platform not initialized")
		return
	}
	caFile := s.Plat.Cfg.MQTT.TLSCA
	if caFile == "" {
		writeErr(w, http.StatusNotFound, "MQTT TLS CA not configured")
		return
	}
	// Resolve relative path: CWD → exe dir → as-is
	path := caFile
	if !filepath.IsAbs(path) {
		if _, err := os.Stat(path); err != nil {
			if exe, err := os.Executable(); err == nil {
				candidate := filepath.Join(filepath.Dir(exe), path)
				if _, err := os.Stat(candidate); err == nil {
					path = candidate
				}
			}
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "read CA: "+err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/x-pem-file")
	w.Header().Set("Content-Disposition", `attachment; filename="ca.pem"`)
	_, _ = w.Write(data)
}

func (s *Server) listProducts(w http.ResponseWriter, r *http.Request) {
	list, err := s.Plat.ListProducts(accountFrom(r.Context()))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) createProduct(w http.ResponseWriter, r *http.Request) {
	var in services.ProductInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	pr, err := s.Plat.CreateProduct(accountFrom(r.Context()), in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	uid := userFrom(r).ID
	s.Plat.Audit(&uid, "user", "create", "product", pr.ProductID, pr)
	writeJSON(w, http.StatusCreated, pr)
}

func (s *Server) getProduct(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	pr, err := s.Plat.GetProductScoped(accountFrom(r.Context()), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, pr)
}

func (s *Server) updateProduct(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if _, err := s.Plat.GetProductScoped(accountFrom(r.Context()), id); err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	var raw map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	var in services.ProductInput
	if v, ok := raw["product_id"]; ok {
		_ = json.Unmarshal(v, &in.ProductID)
	}
	if v, ok := raw["name"]; ok {
		_ = json.Unmarshal(v, &in.Name)
	}
	if v, ok := raw["description"]; ok {
		_ = json.Unmarshal(v, &in.Description)
	}
	if v, ok := raw["topic_style_default"]; ok {
		_ = json.Unmarshal(v, &in.TopicStyleDefault)
	}
	if v, ok := raw["auto_ota"]; ok {
		var b bool
		if err := json.Unmarshal(v, &b); err == nil {
			in.AutoOTA = &b
			in.SetAutoOTA = true
		}
	}
	if v, ok := raw["methods"]; ok {
		in.SetMethods = true
		if err := json.Unmarshal(v, &in.Methods); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid methods: "+err.Error())
			return
		}
	}
	if v, ok := raw["metrics"]; ok {
		in.SetMetrics = true
		if err := json.Unmarshal(v, &in.Metrics); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid metrics: "+err.Error())
			return
		}
	}
	if v, ok := raw["attributes"]; ok {
		in.SetAttributes = true
		if err := json.Unmarshal(v, &in.Attributes); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid attributes: "+err.Error())
			return
		}
	}
	pr, err := s.Plat.UpdateProduct(id, in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, pr)
}

func (s *Server) applyESPDefaults(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if _, err := s.Plat.GetProductScoped(accountFrom(r.Context()), id); err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	pr, err := s.Plat.ApplyProductESPDefaults(id)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, pr)
}

func (s *Server) deleteProduct(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if _, err := s.Plat.GetProductScoped(accountFrom(r.Context()), id); err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	if err := s.Plat.DeleteProduct(id); err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) listDevices(w http.ResponseWriter, r *http.Request) {
	list, err := s.Plat.ListDevices(accountFrom(r.Context()), r.URL.Query().Get("product_id"), r.URL.Query().Get("status"), r.URL.Query().Get("q"))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) createDevice(w http.ResponseWriter, r *http.Request) {
	var in services.DeviceInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	d, creds, err := s.Plat.CreateDevice(accountFrom(r.Context()), in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	uid := userFrom(r).ID
	s.Plat.Audit(&uid, "user", "create", "device", d.ProductID+"."+d.DeviceID, map[string]any{"id": d.ID})
	writeJSON(w, http.StatusCreated, map[string]any{"device": d, "credentials": creds})
}

func (s *Server) getDevice(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	d, err := s.Plat.GetDeviceScoped(accountFrom(r.Context()), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	reported, desired, _ := s.Plat.GetAttributes(id)
	latest, _ := s.Plat.LatestTelemetry(id)
	events, _ := s.Plat.ListEvents(0, id, "", 0, 0, 20)
	product, _ := s.Plat.GetProduct(d.ProductPK)
	dl := protocol.DialectFor(d.TopicStyle)
	username := dl.CredsUsername(d.ProductID, d.DeviceID, "")
	note := dl.ConnectNote()
	var session *models.MQTTClient
	if username != "" {
		session = s.Plat.MQTTClientByUser(username)
	} else {
		// ThingsBoard sessions connect with the raw token as the username.
		session = s.Plat.MQTTClientByTokenPrefix(d.TokenPrefix)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"device": d, "product": product, "reported": reported, "desired": desired, "latest_telemetry": latest, "recent_events": events,
		"connection": map[string]any{
			"client_id":      username,
			"username":       username,
			"broker_host":    s.Plat.Cfg.Platform.BrokerHostPublic,
			"broker_port":    s.Plat.Cfg.MQTT.PublicPort(),
			"broker_ws_url":  s.Plat.Cfg.MQTT.WSPublicURL(s.Plat.Cfg.Platform.BrokerHostPublic),
			"broker_ws_port": s.Plat.Cfg.MQTT.WSPublicPort(),
			"topic_style":    d.TopicStyle,
			"token_prefix":   d.TokenPrefix,
			"note":           note,
			"mqtt_session":   session,
		},
	})
}

func (s *Server) updateDevice(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if _, err := s.Plat.GetDeviceScoped(accountFrom(r.Context()), id); err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	var in services.DeviceInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	d, err := s.Plat.UpdateDevice(id, in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) deleteDevice(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if _, err := s.Plat.GetDeviceScoped(accountFrom(r.Context()), id); err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	if err := s.Plat.DeleteDevice(id); err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) rotateToken(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if _, err := s.Plat.GetDeviceScoped(accountFrom(r.Context()), id); err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	creds, err := s.Plat.RotateDeviceToken(id)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	uid := userFrom(r).ID
	s.Plat.Audit(&uid, "user", "rotate_token", "device", strconv.FormatInt(id, 10), nil)
	writeJSON(w, http.StatusOK, creds)
}

func (s *Server) enableDevice(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if _, err := s.Plat.GetDeviceScoped(accountFrom(r.Context()), id); err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	if err := s.Plat.SetDeviceEnabled(id, true); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) disableDevice(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if _, err := s.Plat.GetDeviceScoped(accountFrom(r.Context()), id); err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	if err := s.Plat.SetDeviceEnabled(id, false); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) deviceTelemetry(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if _, err := s.Plat.GetDeviceScoped(accountFrom(r.Context()), id); err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	from, _ := strconv.ParseInt(r.URL.Query().Get("from"), 10, 64)
	to, _ := strconv.ParseInt(r.URL.Query().Get("to"), 10, 64)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	var keys []string
	if k := r.URL.Query().Get("keys"); k != "" {
		keys = strings.Split(k, ",")
	}
	samples, err := s.Plat.QueryTelemetry(id, keys, from, to, limit)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, samples)
}

func (s *Server) deviceTelemetryLatest(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if _, err := s.Plat.GetDeviceScoped(accountFrom(r.Context()), id); err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	latest, err := s.Plat.LatestTelemetry(id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, latest)
}

func (s *Server) deviceAttributes(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if _, err := s.Plat.GetDeviceScoped(accountFrom(r.Context()), id); err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	reported, desired, err := s.Plat.GetAttributes(id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"reported": reported, "desired": desired})
}

func (s *Server) patchDesired(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	d, err := s.Plat.GetDeviceScoped(accountFrom(r.Context()), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	var attrs map[string]any
	if err := json.NewDecoder(r.Body).Decode(&attrs); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := s.Plat.SetDesired(d, attrs, true); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	uid := userFrom(r).ID
	s.Plat.Audit(&uid, "user", "set_desired", "device", strconv.FormatInt(id, 10), attrs)
	writeJSON(w, http.StatusOK, attrs)
}

func (s *Server) invokeRPC(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	d, err := s.Plat.GetDeviceScoped(accountFrom(r.Context()), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	var req services.RPCRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	wait := r.URL.Query().Get("wait") != "0"
	call, err := s.Plat.InvokeRPC(d, req, wait)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	uid := userFrom(r).ID
	s.Plat.Audit(&uid, "user", "rpc", "device", strconv.FormatInt(id, 10), req)
	writeJSON(w, http.StatusOK, call)
}

func (s *Server) listRPC(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if _, err := s.Plat.GetDeviceScoped(accountFrom(r.Context()), id); err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	list, err := s.Plat.ListRPC(id, 50)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) startOTA(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if _, err := s.Plat.GetDeviceScoped(accountFrom(r.Context()), id); err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	var in struct {
		FirmwareID int64 `json:"firmware_id"`
		Force      bool  `json:"force"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	job, err := s.Plat.StartOTAJob(id, in.FirmwareID, in.Force)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	uid := userFrom(r).ID
	s.Plat.Audit(&uid, "user", "ota", "device", strconv.FormatInt(id, 10), in)
	writeJSON(w, http.StatusOK, job)
}

func (s *Server) notifyFirmware(w http.ResponseWriter, r *http.Request) {
	fwID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var in struct {
		DeviceID int64 `json:"device_id"`
		Force    bool  `json:"force"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	acc := accountFrom(r.Context())
	if !s.Plat.FirmwareInAccount(acc, fwID) {
		writeErr(w, http.StatusNotFound, "firmware not found")
		return
	}
	if in.DeviceID > 0 {
		if _, err := s.Plat.GetDeviceScoped(acc, in.DeviceID); err != nil {
			writeErr(w, http.StatusNotFound, err.Error())
			return
		}
	}
	out, err := s.Plat.NotifyFirmware(fwID, in.DeviceID, in.Force)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	uid := userFrom(r).ID
	s.Plat.Audit(&uid, "user", "ota_notify", "firmware", strconv.FormatInt(fwID, 10), in)
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) cancelOTA(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	d, err := s.Plat.GetDeviceScoped(accountFrom(r.Context()), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	var in struct {
		Module string `json:"module"`
		Reason string `json:"reason"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	if in.Reason == "" {
		in.Reason = "cancelled by operator"
	}
	if err := s.Plat.CancelOTA(d, in.Module, in.Reason); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) listEvents(w http.ResponseWriter, r *http.Request) {
	devicePK, _ := strconv.ParseInt(r.URL.Query().Get("device_id"), 10, 64)
	from, _ := strconv.ParseInt(r.URL.Query().Get("from"), 10, 64)
	to, _ := strconv.ParseInt(r.URL.Query().Get("to"), 10, 64)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	list, err := s.Plat.ListEvents(accountFrom(r.Context()), devicePK, r.URL.Query().Get("severity"), from, to, limit)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) listLogs(w http.ResponseWriter, r *http.Request) {
	devicePK, _ := strconv.ParseInt(r.URL.Query().Get("device_id"), 10, 64)
	from, _ := strconv.ParseInt(r.URL.Query().Get("from"), 10, 64)
	to, _ := strconv.ParseInt(r.URL.Query().Get("to"), 10, 64)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	list, err := s.Plat.ListLogs(accountFrom(r.Context()), devicePK, r.URL.Query().Get("level"), from, to, limit)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) listFirmware(w http.ResponseWriter, r *http.Request) {
	list, err := s.Plat.ListFirmware(accountFrom(r.Context()), r.URL.Query().Get("product_id"))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) registerFirmware(w http.ResponseWriter, r *http.Request) {
	var in services.FirmwareInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	fw, err := s.Plat.RegisterFirmware(accountFrom(r.Context()), in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, fw)
}

func (s *Server) deleteFirmware(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if !s.Plat.FirmwareInAccount(accountFrom(r.Context()), id) {
		writeErr(w, http.StatusNotFound, "firmware not found")
		return
	}
	if err := s.Plat.DeleteFirmware(id); err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) uploadFirmware(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, services.MaxFirmwareBytes+1<<20)
	if err := r.ParseMultipartForm(services.MaxFirmwareBytes + 1<<20); err != nil {
		writeErr(w, http.StatusBadRequest, "file too large or invalid multipart")
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "file required")
		return
	}
	defer file.Close()
	in := services.FirmwareInput{
		ProductID:  r.FormValue("product_id"),
		Module:     r.FormValue("module"),
		Version:    r.FormValue("version"),
		SHA256:     r.FormValue("sha256"),
		SignMethod: r.FormValue("sign_method"),
		Sign:       r.FormValue("sign"),
	}
	if sz := strings.TrimSpace(r.FormValue("size")); sz != "" {
		in.Size, _ = strconv.ParseInt(sz, 10, 64)
	}
	fw, err := s.Plat.StoreFirmwareFile(accountFrom(r.Context()), in, file)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, fw)
}

func (s *Server) downloadFirmware(w http.ResponseWriter, r *http.Request) {
	if s.Plat == nil {
		// Setup mode
		http.NotFound(w, r)
		return
	}
	fw, path, err := s.Plat.HostedFirmware(chi.URLParam(r, "token"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	name := strings.NewReplacer(`"`, "", `/`, "-", `\`, "-").Replace(fw.ProductID+"-"+fw.Module+"-"+fw.Version) + ".bin"
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	http.ServeFile(w, r, path)
}

func (s *Server) listOTAJobs(w http.ResponseWriter, r *http.Request) {
	devicePK, _ := strconv.ParseInt(r.URL.Query().Get("device_id"), 10, 64)
	list, err := s.Plat.ListOTAJobs(accountFrom(r.Context()), devicePK)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) listAuditLog(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	list, err := s.Plat.ListAuditLog(accountFrom(r.Context()), limit)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"platform_name":            s.Plat.Cfg.Platform.Name,
		"telemetry_retention_days": s.Plat.Cfg.Platform.TelemetryRetentionDays,
		"rpc_default_timeout_ms":   int(s.Plat.Cfg.Platform.RPCDefaultTimeout / time.Millisecond),
		"rules_bus_enabled":        s.Plat.Cfg.Rules.Enabled,
		"rules_bus_prefix":         s.Plat.Cfg.Rules.Prefix,
		"broker_host":              s.Plat.Cfg.Platform.BrokerHostPublic,
		"mqtt_port":                s.Plat.Cfg.MQTT.PublicPort(),
		"mqtt_ws_url":              s.Plat.Cfg.MQTT.WSPublicURL(s.Plat.Cfg.Platform.BrokerHostPublic),
		"mqtt_ws_port":             s.Plat.Cfg.MQTT.WSPublicPort(),
		"register_enabled":         s.Plat.Cfg.Register.Enabled,
		"require_email_verify":     s.Plat.Cfg.Register.RequireVerify,
		"google_enabled":           s.Plat.Cfg.GoogleEnabled(),
		"public_base_url":          s.Plat.Cfg.Platform.PublicBaseURL,
		"timezone":                 s.Plat.Cfg.TimezoneOrUTC(),
		"version":                  s.Version,
		"commit":                   s.Commit,
	})
}
