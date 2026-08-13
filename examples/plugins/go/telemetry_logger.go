// Telemetry Logger — Webhook External Plugin
//
// Receives telemetry events from the platform via HTTP POST (JSON-RPC)
// and logs them to console. Sends periodic heartbeats.
//
// Usage:
//
//	go run telemetry_logger.go --platform http://localhost:8080 --token <admin-jwt> --port 9000
//
// Dependencies: none (stdlib only)
//
// The plugin:
// 1. Registers with the platform using admin JWT
// 2. Starts HTTP server on specified port
// 3. Receives plugin.on_event, plugin.apply_config, plugin.detach via JSON-RPC
// 4. Sends heartbeat every 30 seconds
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

// ─── Configuration ───────────────────────────────────────────────────────────

const (
	pluginID   = "telemetry-logger"
	pluginName = "Telemetry Logger"
	pluginDesc = "Logs device telemetry to console via webhook"
)

// ─── Globals ─────────────────────────────────────────────────────────────────

var (
	pluginToken string
	eventsCount int64
	startTime   = time.Now()
	mu          sync.Mutex
)

// ─── Registration ────────────────────────────────────────────────────────────

func register(platformURL, jwtToken, endpoint string) (string, error) {
	payload := map[string]any{
		"id":             pluginID,
		"name":           pluginName,
		"description":    pluginDesc,
		"endpoint":       endpoint,
		"auto":           false,
		"interval_ms":    0,
		"default_config": map[string]any{},
		"form":           []any{},
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST", platformURL+"/api/v1/plugins/external", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+jwtToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("registration failed: %d", resp.StatusCode)
	}

	var result struct {
		OK    bool   `json:"ok"`
		ID    string `json:"id"`
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	log.Printf("Registered: id=%s token=%s...", result.ID, result.Token[:16])
	return result.Token, nil
}

// ─── Heartbeat ───────────────────────────────────────────────────────────────

func heartbeatLoop(platformURL string) {
	for {
		time.Sleep(30 * time.Second)

		mu.Lock()
		count := eventsCount
		mu.Unlock()

		payload := map[string]any{
			"jsonrpc": "2.0",
			"id":      1,
			"method":  "platform.heartbeat",
			"params": map[string]any{
				"plugin_id": pluginID,
				"status":    "ok",
				"stats": map[string]any{
					"events_processed": count,
					"uptime_sec":       int(time.Since(startTime).Seconds()),
				},
			},
		}
		body, _ := json.Marshal(payload)

		req, _ := http.NewRequest("POST", platformURL+"/api/plugins/external/rpc", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+pluginToken)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			log.Printf("Heartbeat failed: %v", err)
			continue
		}
		resp.Body.Close()

		if resp.StatusCode == 200 {
			log.Printf("Heartbeat OK (events: %d)", count)
		} else {
			log.Printf("Heartbeat failed: %d", resp.StatusCode)
		}
	}
}

// ─── JSON-RPC Types ──────────────────────────────────────────────────────────

type jsonrpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type jsonrpcResponse struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id"`
	Result  any    `json:"result,omitempty"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// ─── HTTP Handler ────────────────────────────────────────────────────────────

func handleRPC(w http.ResponseWriter, r *http.Request) {
	var req jsonrpcRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, nil, -32700, "parse error")
		return
	}

	switch req.Method {
	case "plugin.on_event":
		handleOnEvent(w, req)
	case "plugin.apply_config":
		handleApplyConfig(w, req)
	case "plugin.detach":
		handleDetach(w, req)
	default:
		writeError(w, req.ID, -32601, "unknown method: "+req.Method)
	}
}

func handleOnEvent(w http.ResponseWriter, req jsonrpcRequest) {
	mu.Lock()
	eventsCount++
	mu.Unlock()

	var params struct {
		BindingID int64  `json:"binding_id"`
		DevicePK  int64  `json:"device_pk"`
		Kind      string `json:"kind"`
		Online    bool   `json:"online"`
		Payload   any    `json:"payload"`
	}
	_ = json.Unmarshal(req.Params, &params)

	switch params.Kind {
	case "telemetry":
		log.Printf("📊 Device #%d (binding %d):", params.DevicePK, params.BindingID)
		if m, ok := params.Payload.(map[string]any); ok {
			for k, v := range m {
				log.Printf("   %s: %v", k, v)
			}
		}
	case "status":
		status := "offline"
		if params.Online {
			status = "online"
		}
		log.Printf("🔌 Device #%d: %s", params.DevicePK, status)
	case "event":
		log.Printf("⚡ Device #%d event: %v", params.DevicePK, params.Payload)
	default:
		log.Printf("📨 Device #%d [%s]: %v", params.DevicePK, params.Kind, params.Payload)
	}

	writeResult(w, req.ID, map[string]any{"ok": true})
}

func handleApplyConfig(w http.ResponseWriter, req jsonrpcRequest) {
	var params struct {
		BindingID int64 `json:"binding_id"`
	}
	_ = json.Unmarshal(req.Params, &params)
	log.Printf("Binding %d: config applied", params.BindingID)
	writeResult(w, req.ID, map[string]any{"ok": true})
}

func handleDetach(w http.ResponseWriter, req jsonrpcRequest) {
	var params struct {
		BindingID int64 `json:"binding_id"`
	}
	_ = json.Unmarshal(req.Params, &params)
	log.Printf("Binding %d: detached", params.BindingID)
	writeResult(w, req.ID, map[string]any{"ok": true})
}

// ─── JSON-RPC Helpers ────────────────────────────────────────────────────────

func writeResult(w http.ResponseWriter, id any, result any) {
	json.NewEncoder(w).Encode(jsonrpcResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	})
}

func writeError(w http.ResponseWriter, id any, code int, message string) {
	json.NewEncoder(w).Encode(jsonrpcResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		}{Code: code, Message: message},
	})
}

// ─── Main ────────────────────────────────────────────────────────────────────

func main() {
	platformURL := flag.String("platform", "http://localhost:8080", "Platform URL")
	token := flag.String("token", "", "Admin JWT token for registration (required)")
	port := flag.Int("port", 9000, "HTTP server port for webhook callbacks")
	flag.Parse()

	if *token == "" {
		log.Fatal("--token is required (admin JWT)")
	}

	log.Println("==================================================")
	log.Println("Telemetry Logger Plugin (Webhook)")
	log.Println("==================================================")

	// Determine endpoint
	hostname, _ := os.Hostname()
	endpoint := fmt.Sprintf("http://%s:%d/rpc", hostname, *port)

	// Register
	var err error
	pluginToken, err = register(*platformURL, *token, endpoint)
	if err != nil {
		log.Fatalf("Registration failed: %v", err)
	}

	// Start heartbeat
	go heartbeatLoop(*platformURL)

	// Setup HTTP server
	mux := http.NewServeMux()
	mux.HandleFunc("/rpc", handleRPC)

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", *port),
		Handler: mux,
	}

	// Graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Println("Shutting down...")
		server.Close()
	}()

	log.Printf("Listening on port %d...", *port)
	log.Println("Waiting for events from platform...")
	log.Println("--------------------------------------------------")

	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("HTTP server error: %v", err)
	}
}
