package api

import (
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
	"cogitor-iot-platform/internal/services"
	"cogitor-iot-platform/internal/ws"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Server struct {
	Plat  *services.Platform
	Hub   *ws.Hub
	SpaFS fs.FS // optional embedded SPA (dist contents)
	Setup *services.SetupService // setup wizard service (nil if setup not needed)
}

func (s *Server) Router() http.Handler {
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
		mqttOK := s.Plat.Pub != nil && s.Plat.Pub.Connected()
		status := http.StatusOK
		if !mqttOK {
			status = http.StatusServiceUnavailable
		}
		st := s.Plat.MQTTStatus()
		writeJSON(w, status, map[string]any{
			"db":           true,
			"mqtt":         mqttOK,
			"mqtt_addr":    s.Plat.Cfg.MQTTAddr,
			"mqtt_clients": st.ClientCount,
		})
	})

	// Public: CA certificate for MQTT TLS clients (no auth required — needed before connect)
	r.Get("/api/v1/mqtt/ca.pem", s.mqttCACert)

	// Setup wizard endpoints (public, no auth)
	if s.Setup != nil {
		setupHandler := &SetupHandler{Setup: s.Setup}
		r.Route("/api/v1/setup", func(r chi.Router) {
			r.Get("/state", setupHandler.GetState)
			r.Post("/step", setupHandler.UpdateStep)
			r.Post("/apply", setupHandler.Apply)
		})
	}

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/auth/config", s.authConfig)
		r.Post("/auth/login", s.login)
		r.Post("/auth/register", s.register)
		r.Get("/auth/verify-email", s.verifyEmail)
		r.Post("/auth/resend-verification", s.resendVerification)
		r.Get("/auth/google", s.googleStart)
		r.Get("/auth/google/callback", s.googleCallback)

		r.Group(func(r chi.Router) {
			r.Use(s.authenticate)
			r.Get("/overview", s.overview)
			r.Get("/mqtt", s.mqttStatus)
			r.Get("/me", s.me)
			r.Get("/me/tokens", s.listAPITokens)
			r.Post("/me/tokens", s.createAPIToken)
			r.Delete("/me/tokens/{id}", s.revokeAPIToken)

			r.Get("/users", s.requireAdmin(s.listUsers))
			r.Patch("/users/{id}", s.requireAdmin(s.updateUser))
			r.Delete("/users/{id}", s.requireAdmin(s.deleteUser))

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

			r.Get("/ws", s.Hub.ServeWS)
		})
	})

	// Embedded Streamable HTTP MCP (same process as platform — not a separate deploy).
	mcpHandler := mcp.NewStreamableHTTPHandler(func(req *http.Request) *mcp.Server {
		u := userFrom(req)
		role := models.Role("")
		if u != nil {
			role = u.Role
		}
		return mcpbridge.NewServer(mcpbridge.NewLocal(s.Plat, role))
	}, &mcp.StreamableHTTPOptions{Stateless: true})
	r.Group(func(r chi.Router) {
		r.Use(s.authenticate)
		r.Handle("/mcp", mcpHandler)
		r.Handle("/mcp/*", mcpHandler)
	})

	// External plugin RPC endpoint (plugin token auth, not JWT)
	r.Post("/api/plugins/external/rpc", s.externalPluginRPC)

	if s.SpaFS != nil {
		r.Handle("/*", spaHandler(s.SpaFS))
	}
	return r
}

type ctxKey int

const userKey ctxKey = 1

func (s *Server) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tok := auth.Bearer(r)
		if tok == "" {
			tok = r.URL.Query().Get("token")
		}
		if tok == "" {
			writeErr(w, http.StatusUnauthorized, "missing token")
			return
		}
		claims, err := auth.ParseJWT(s.Plat.Cfg.JWTSecret, tok)
		if err == nil {
			u, err := s.Plat.GetUser(claims.UserID)
			if err != nil || u.Disabled {
				writeErr(w, http.StatusUnauthorized, "invalid token")
				return
			}
			next.ServeHTTP(w, r.WithContext(withUser(r.Context(), *u)))
			return
		}
		u, err := s.Plat.UserByAPIToken(tok)
		if err != nil || u.Disabled {
			writeErr(w, http.StatusUnauthorized, "invalid token")
			return
		}
		next.ServeHTTP(w, r.WithContext(withUser(r.Context(), *u)))
	})
}

func (s *Server) requireWrite(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u := userFrom(r)
		if u == nil || !auth.CanWrite(u.Role) {
			writeErr(w, http.StatusForbidden, "write access required")
			return
		}
		h(w, r)
	}
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, userFrom(r))
}

func (s *Server) overview(w http.ResponseWriter, r *http.Request) {
	o, err := s.Plat.Overview()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, o)
}

func (s *Server) mqttStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.Plat.MQTTStatus())
}

// mqttCACert serves the MQTT TLS CA certificate (public, no auth).
func (s *Server) mqttCACert(w http.ResponseWriter, r *http.Request) {
	caFile := s.Plat.Cfg.MQTTTLSCA
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
	list, err := s.Plat.ListProducts()
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
	pr, err := s.Plat.CreateProduct(in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	uid := userFrom(r).ID
	s.Plat.Audit(&uid, "create", "product", pr.ProductID, pr)
	writeJSON(w, http.StatusCreated, pr)
}

func (s *Server) getProduct(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	pr, err := s.Plat.GetProduct(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, pr)
}

func (s *Server) updateProduct(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
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
	pr, err := s.Plat.ApplyProductESPDefaults(id)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, pr)
}

func (s *Server) deleteProduct(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err := s.Plat.DeleteProduct(id); err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) listDevices(w http.ResponseWriter, r *http.Request) {
	list, err := s.Plat.ListDevices(r.URL.Query().Get("product_id"), r.URL.Query().Get("status"), r.URL.Query().Get("q"))
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
	d, creds, err := s.Plat.CreateDevice(in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	uid := userFrom(r).ID
	s.Plat.Audit(&uid, "create", "device", d.ProductID+"."+d.DeviceID, map[string]any{"id": d.ID})
	writeJSON(w, http.StatusCreated, map[string]any{"device": d, "credentials": creds})
}

func (s *Server) getDevice(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	d, err := s.Plat.GetDevice(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	reported, desired, _ := s.Plat.GetAttributes(id)
	latest, _ := s.Plat.LatestTelemetry(id)
	events, _ := s.Plat.ListEvents(id, "", 0, 0, 20)
	product, _ := s.Plat.GetProduct(d.ProductPK)
	username := d.ProductID + "." + d.DeviceID
	writeJSON(w, http.StatusOK, map[string]any{
		"device": d, "product": product, "reported": reported, "desired": desired, "latest_telemetry": latest, "recent_events": events,
		"connection": map[string]any{
			"client_id":      username,
			"username":       username,
			"broker_host":    s.Plat.Cfg.BrokerHostPublic,
			"broker_port":    s.Plat.Cfg.MQTTPublicPort(),
			"broker_ws_url":  s.Plat.Cfg.MQTTWSPublicURL(),
			"broker_ws_port": s.Plat.Cfg.MQTTWSPublicPort(),
			"topic_style":    d.TopicStyle,
			"token_prefix":   d.TokenPrefix,
			"note":           "MQTT username = product.device; password = access token (shown only at create/rotate). TCP :1883 or WebSocket MQTT_WS_ADDR (default :8083).",
			"mqtt_session":   s.Plat.MQTTClientByUser(username),
		},
	})
}

func (s *Server) updateDevice(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
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
	if err := s.Plat.DeleteDevice(id); err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) rotateToken(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	creds, err := s.Plat.RotateDeviceToken(id)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	uid := userFrom(r).ID
	s.Plat.Audit(&uid, "rotate_token", "device", strconv.FormatInt(id, 10), nil)
	writeJSON(w, http.StatusOK, creds)
}

func (s *Server) enableDevice(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err := s.Plat.SetDeviceEnabled(id, true); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) disableDevice(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err := s.Plat.SetDeviceEnabled(id, false); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) deviceTelemetry(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
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
	latest, err := s.Plat.LatestTelemetry(id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, latest)
}

func (s *Server) deviceAttributes(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	reported, desired, err := s.Plat.GetAttributes(id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"reported": reported, "desired": desired})
}

func (s *Server) patchDesired(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	d, err := s.Plat.GetDevice(id)
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
	s.Plat.Audit(&uid, "set_desired", "device", strconv.FormatInt(id, 10), attrs)
	writeJSON(w, http.StatusOK, attrs)
}

func (s *Server) invokeRPC(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	d, err := s.Plat.GetDevice(id)
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
	s.Plat.Audit(&uid, "rpc", "device", strconv.FormatInt(id, 10), req)
	writeJSON(w, http.StatusOK, call)
}

func (s *Server) listRPC(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	list, err := s.Plat.ListRPC(id, 50)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) startOTA(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
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
	s.Plat.Audit(&uid, "ota", "device", strconv.FormatInt(id, 10), in)
	writeJSON(w, http.StatusOK, job)
}

func (s *Server) notifyFirmware(w http.ResponseWriter, r *http.Request) {
	fwID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var in struct {
		DeviceID int64 `json:"device_id"`
		Force    bool  `json:"force"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	out, err := s.Plat.NotifyFirmware(fwID, in.DeviceID, in.Force)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	uid := userFrom(r).ID
	s.Plat.Audit(&uid, "ota_notify", "firmware", strconv.FormatInt(fwID, 10), in)
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) cancelOTA(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	d, err := s.Plat.GetDevice(id)
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
	list, err := s.Plat.ListEvents(devicePK, r.URL.Query().Get("severity"), from, to, limit)
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
	list, err := s.Plat.ListLogs(devicePK, r.URL.Query().Get("level"), from, to, limit)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) listFirmware(w http.ResponseWriter, r *http.Request) {
	list, err := s.Plat.ListFirmware(r.URL.Query().Get("product_id"))
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
	fw, err := s.Plat.RegisterFirmware(in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, fw)
}

func (s *Server) deleteFirmware(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
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
	fw, err := s.Plat.StoreFirmwareFile(in, file)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, fw)
}

func (s *Server) downloadFirmware(w http.ResponseWriter, r *http.Request) {
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
	list, err := s.Plat.ListOTAJobs(devicePK)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"platform_name":            s.Plat.Cfg.PlatformName,
		"telemetry_retention_days": s.Plat.Cfg.TelemetryRetentionDays,
		"rpc_default_timeout_ms":   int(s.Plat.Cfg.RPCDefaultTimeout / time.Millisecond),
		"nodered_url":              s.Plat.Cfg.NodeRedURL,
		"rules_bus_enabled":        s.Plat.Cfg.RulesBusEnabled,
		"broker_host":              s.Plat.Cfg.BrokerHostPublic,
		"mqtt_port":                s.Plat.Cfg.MQTTPublicPort(),
		"mqtt_ws_url":              s.Plat.Cfg.MQTTWSPublicURL(),
		"mqtt_ws_port":             s.Plat.Cfg.MQTTWSPublicPort(),
		"register_enabled":         s.Plat.Cfg.RegisterEnabled,
		"require_email_verify":     s.Plat.Cfg.RequireEmailVerify,
		"google_enabled":           s.Plat.Cfg.GoogleEnabled(),
		"public_base_url":          s.Plat.Cfg.PublicBaseURL,
		"timezone":                 s.Plat.Cfg.TimezoneOrUTC(),
	})
}
