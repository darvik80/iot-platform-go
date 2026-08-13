// MQTT Telemetry Logger — External Plugin (Node-RED style)
//
// Connects to the platform MQTT broker directly, subscribes to rules bus events,
// and logs all telemetry/status/events to console.
//
// Usage:
//
//	go run mqtt_telemetry_logger.go --platform http://localhost:8080 --token <admin-jwt>
//
// Dependencies:
//
//	go get github.com/eclipse/paho.mqtt.golang
//
// The plugin:
// 1. Registers with the platform using admin JWT (gets API token + MQTT credentials)
// 2. Connects to MQTT broker using the provided credentials
// 3. Subscribes to platform/v1/events/# (all device events)
// 4. Logs everything to console
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// ─── Configuration ───────────────────────────────────────────────────────────

const (
	pluginID   = "mqtt-telemetry-logger"
	pluginName = "MQTT Telemetry Logger"
	pluginDesc = "Subscribes to rules bus via MQTT and logs all device telemetry"
)

// ─── Registration ────────────────────────────────────────────────────────────

type RegistrationResponse struct {
	OK       bool   `json:"ok"`
	ID       string `json:"id"`
	Token    string `json:"token"`
	MQTTUser string `json:"mqtt_user"`
	MQTTPass string `json:"mqtt_pass"`
	MQTTHost string `json:"mqtt_host"`
	MQTTPort int    `json:"mqtt_port"`
	RulesBus string `json:"rules_bus"`
}

func register(platformURL, jwtToken string) (*RegistrationResponse, error) {
	payload := map[string]string{
		"id":          pluginID,
		"name":        pluginName,
		"description": pluginDesc,
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST", platformURL+"/api/v1/plugins/external", strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+jwtToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("registration failed: %d", resp.StatusCode)
	}

	var result RegistrationResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	log.Printf("Registered: id=%s mqtt_user=%s", result.ID, result.MQTTUser)
	return &result, nil
}

// ─── MQTT Client ─────────────────────────────────────────────────────────────

type MQTTLoggerPlugin struct {
	config   *RegistrationResponse
	client   mqtt.Client
	rulesBus string
}

func NewMQTTLoggerPlugin(config *RegistrationResponse) *MQTTLoggerPlugin {
	return &MQTTLoggerPlugin{
		config:   config,
		rulesBus: config.RulesBus,
	}
}

func (p *MQTTLoggerPlugin) Connect() error {
	clientID := fmt.Sprintf("extplg_%s", pluginID)
	broker := fmt.Sprintf("tcp://%s:%d", p.config.MQTTHost, p.config.MQTTPort)

	opts := mqtt.NewClientOptions()
	opts.AddBroker(broker)
	opts.SetClientID(clientID)
	opts.SetUsername(p.config.MQTTUser)
	opts.SetPassword(p.config.MQTTPass)
	opts.SetKeepAlive(60 * time.Second)
	opts.SetAutoReconnect(true)
	opts.SetOnConnectHandler(p.onConnect)
	opts.SetConnectionLostHandler(p.onConnectionLost)

	p.client = mqtt.NewClient(opts)

	log.Printf("Connecting to MQTT broker %s as %s", broker, p.config.MQTTUser)
	if token := p.client.Connect(); token.Wait() && token.Error() != nil {
		return token.Error()
	}

	return nil
}

func (p *MQTTLoggerPlugin) onConnect(client mqtt.Client) {
	log.Println("MQTT connected")

	// Subscribe to all rules bus events
	topic := p.rulesBus + "/events/#"
	if token := client.Subscribe(topic, 0, p.onMessage); token.Wait() && token.Error() != nil {
		log.Printf("Subscribe failed: %v", token.Error())
		return
	}
	log.Printf("Subscribed to %s", topic)
}

func (p *MQTTLoggerPlugin) onConnectionLost(_ mqtt.Client, err error) {
	log.Printf("MQTT disconnected: %v", err)
}

func (p *MQTTLoggerPlugin) onMessage(_ mqtt.Client, msg mqtt.Message) {
	topic := msg.Topic()
	payload := msg.Payload()

	// Parse topic: platform/v1/events/{product}/{device}/{kind}
	parts := strings.Split(topic, "/")
	kind := "unknown"
	device := "unknown"
	if len(parts) > 3 {
		kind = parts[len(parts)-1]
		device = parts[len(parts)-2]
	} else if len(parts) > 0 {
		device = parts[len(parts)-1]
	}

	ts := time.Now().Format("15:04:05")

	// Try to parse payload as JSON
	var payloadObj any
	if err := json.Unmarshal(payload, &payloadObj); err != nil {
		payloadObj = string(payload)
	}

	switch kind {
	case "telemetry":
		if m, ok := payloadObj.(map[string]any); ok {
			if metrics, exists := m["metrics"]; exists {
				payloadObj = metrics
			}
		}
		jsonBytes, _ := json.Marshal(payloadObj)
		log.Printf("📊 [%s] %s telemetry: %s", ts, device, string(jsonBytes))

	case "status":
		var online any = "?"
		if m, ok := payloadObj.(map[string]any); ok {
			if v, exists := m["online"]; exists {
				online = v
			}
		}
		log.Printf("🔌 [%s] %s status: %v", ts, device, online)

	case "event":
		jsonBytes, _ := json.Marshal(payloadObj)
		log.Printf("⚡ [%s] %s event: %s", ts, device, string(jsonBytes))

	case "log":
		log.Printf("📝 [%s] %s log: %v", ts, device, payloadObj)

	case "ota":
		jsonBytes, _ := json.Marshal(payloadObj)
		log.Printf("📦 [%s] %s OTA: %s", ts, device, string(jsonBytes))

	default:
		jsonBytes, _ := json.Marshal(payloadObj)
		log.Printf("📨 [%s] %s/%s: %s", ts, device, kind, string(jsonBytes))
	}
}

func (p *MQTTLoggerPlugin) Stop() {
	if p.client != nil && p.client.IsConnected() {
		p.client.Disconnect(250)
		log.Println("MQTT disconnected")
	}
}

// ─── Main ────────────────────────────────────────────────────────────────────

func main() {
	platformURL := flag.String("platform", "http://localhost:8080", "Platform URL")
	token := flag.String("token", "", "Admin JWT token for registration (required)")
	flag.Parse()

	if *token == "" {
		log.Fatal("--token is required (admin JWT)")
	}

	// Register
	config, err := register(*platformURL, *token)
	if err != nil {
		log.Fatalf("Registration failed: %v", err)
	}

	// Create plugin
	plugin := NewMQTTLoggerPlugin(config)

	// Graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Connect
	if err := plugin.Connect(); err != nil {
		log.Fatalf("MQTT connect failed: %v", err)
	}

	// Wait for signal
	<-sigCh
	log.Println("Shutting down...")
	plugin.Stop()
}
