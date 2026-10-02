// MQTT Telemetry Logger — External Plugin (Node-RED style)
//
// Connects to the platform MQTT broker directly, subscribes to rules bus events,
// and logs all telemetry/status/events to console.
//
// Usage:
//
//	# With admin JWT (registers plugin, gets credentials)
//	go run mqtt_telemetry_logger.go --platform http://localhost:8080 --token <admin-jwt>
//
//	# With existing credentials
//	go run mqtt_telemetry_logger.go --mqtt-user extplg_acme_mqtt-telemetry-logger --mqtt-pass <password>
//
//	# With environment variables
//	MQTT_USER=extplg_acme_mqtt-telemetry-logger MQTT_PASS=<password> go run mqtt_telemetry_logger.go
//
// Dependencies:
//
//	go get github.com/eclipse/paho.mqtt.golang
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
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

// ─── Config ──────────────────────────────────────────────────────────────────

type PluginConfig struct {
	MQTTUser string
	MQTTPass string
	MQTTHost string
	MQTTPort int
	RulesBus string
}

// ─── Registration ────────────────────────────────────────────────────────────

type RegistrationResponse struct {
	OK       bool   `json:"ok"`
	ID       string `json:"id"`
	MQTTUser string `json:"mqtt_user"`
	MQTTPass string `json:"mqtt_pass"`
	MQTTHost string `json:"mqtt_host"`
	MQTTPort int    `json:"mqtt_port"`
	RulesBus string `json:"rules_bus"`
}

func register(platformURL, jwtToken string) (*PluginConfig, error) {
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
	return &PluginConfig{
		MQTTUser: result.MQTTUser,
		MQTTPass: result.MQTTPass,
		MQTTHost: result.MQTTHost,
		MQTTPort: result.MQTTPort,
		RulesBus: result.RulesBus,
	}, nil
}

// ─── MQTT Client ─────────────────────────────────────────────────────────────

type MQTTLoggerPlugin struct {
	config     *PluginConfig
	client     mqtt.Client
	eventsRoot string // {rules_bus}/events/{account_slug}/
}

// accountSlug extracts the account from extplg_{account_slug}_{plugin_id}.
func accountSlug(mqttUser string) (string, error) {
	rest, ok := strings.CutPrefix(mqttUser, "extplg_")
	slug, _, found := strings.Cut(rest, "_")
	if !ok || !found || slug == "" {
		return "", fmt.Errorf("not an external plugin user: %q", mqttUser)
	}
	return slug, nil
}

func NewMQTTLoggerPlugin(config *PluginConfig) (*MQTTLoggerPlugin, error) {
	slug, err := accountSlug(config.MQTTUser)
	if err != nil {
		return nil, err
	}
	// The ACL only allows subscribing inside the plugin's own account.
	return &MQTTLoggerPlugin{
		config:     config,
		eventsRoot: config.RulesBus + "/events/" + slug + "/",
	}, nil
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

	topic := p.eventsRoot + "#"
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

	// {product}/{device}/{kind}; kind may contain a slash (rpc/result, attributes/reported).
	parts := strings.SplitN(strings.TrimPrefix(topic, p.eventsRoot), "/", 3)
	if len(parts) < 3 {
		log.Printf("Unexpected topic: %s", topic)
		return
	}
	device := parts[0] + "/" + parts[1]
	kind := parts[2]

	ts := time.Now().Format("15:04:05")

	var payloadObj any
	if err := json.Unmarshal(payload, &payloadObj); err != nil {
		payloadObj = string(payload)
	}

	switch kind {
	case "telemetry":
		if m, ok := payloadObj.(map[string]any); ok {
			if keys, exists := m["keys"]; exists {
				payloadObj = keys
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
	token := flag.String("token", "", "Admin JWT token for registration")
	mqttUser := flag.String("mqtt-user", "", "MQTT username (or MQTT_USER env)")
	mqttPass := flag.String("mqtt-pass", "", "MQTT password (or MQTT_PASS env)")
	mqttHost := flag.String("mqtt-host", "localhost", "MQTT broker host")
	mqttPort := flag.Int("mqtt-port", 1883, "MQTT broker port")
	rulesBus := flag.String("rules-bus", "platform/v1", "Rules bus prefix")
	flag.Parse()

	var config *PluginConfig
	var err error

	switch {
	case *token != "":
		// Registration mode
		config, err = register(*platformURL, *token)
		if err != nil {
			log.Fatalf("Registration failed: %v", err)
		}

	case *mqttUser != "" && *mqttPass != "":
		// Direct credentials from args
		config = &PluginConfig{
			MQTTUser: *mqttUser,
			MQTTPass: *mqttPass,
			MQTTHost: *mqttHost,
			MQTTPort: *mqttPort,
			RulesBus: *rulesBus,
		}
		log.Printf("Using direct credentials for %s", *mqttUser)

	case os.Getenv("MQTT_USER") != "" && os.Getenv("MQTT_PASS") != "":
		// Direct credentials from env
		port := *mqttPort
		if p := os.Getenv("MQTT_PORT"); p != "" {
			if v, err := strconv.Atoi(p); err == nil {
				port = v
			}
		}
		rules := *rulesBus
		if r := os.Getenv("RULES_BUS"); r != "" {
			rules = r
		}
		config = &PluginConfig{
			MQTTUser: os.Getenv("MQTT_USER"),
			MQTTPass: os.Getenv("MQTT_PASS"),
			MQTTHost: getEnvOrDefault("MQTT_HOST", *mqttHost),
			MQTTPort: port,
			RulesBus: rules,
		}
		log.Printf("Using credentials from environment for %s", config.MQTTUser)

	default:
		log.Fatal("Either --token or --mqtt-user/--mqtt-pass (or env vars) required")
	}

	plugin, err := NewMQTTLoggerPlugin(config)
	if err != nil {
		log.Fatal(err)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	if err := plugin.Connect(); err != nil {
		log.Fatalf("MQTT connect failed: %v", err)
	}

	<-sigCh
	log.Println("Shutting down...")
	plugin.Stop()
}

func getEnvOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
