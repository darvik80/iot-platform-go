package mqtt

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	pahomqtt "github.com/eclipse/paho.mqtt.golang"
	amqp "github.com/rabbitmq/amqp091-go"

	"crearts-iot-platform/internal/config"
	"crearts-iot-platform/internal/db"
	"crearts-iot-platform/internal/models"
	"crearts-iot-platform/internal/services"
)

type Client struct {
	cfg       config.Config
	plat      *services.Platform
	log       *slog.Logger
	mqtt      pahomqtt.Client
	amqpConn  *amqp.Connection
	amqpCh    *amqp.Channel
	connected atomic.Bool
	mu        sync.Mutex
	stop      chan struct{}
}

func New(cfg config.Config, plat *services.Platform, log *slog.Logger) *Client {
	return &Client{cfg: cfg, plat: plat, log: log, stop: make(chan struct{})}
}

func (c *Client) Connected() bool { return c.connected.Load() }

func (c *Client) Start() error {
	go func() {
		if err := c.startMQTT(); err != nil {
			c.log.Warn("mqtt publish client failed", "err", err)
		}
	}()
	if err := c.startAMQPIngest(); err != nil {
		c.log.Warn("amqp ingest unavailable", "err", err)
		// retry loop in background
		go c.reconnectAMQP()
		return err
	}
	return nil
}

func (c *Client) reconnectAMQP() {
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-c.stop:
			return
		case <-t.C:
			if err := c.startAMQPIngest(); err == nil {
				c.log.Info("amqp ingest reconnected")
				return
			}
		}
	}
}

func (c *Client) startMQTT() error {
	opts := pahomqtt.NewClientOptions()
	opts.AddBroker(c.cfg.MQTTBroker)
	opts.SetClientID(c.cfg.MQTTClientID)
	opts.SetUsername(c.cfg.MQTTUsername)
	opts.SetPassword(c.cfg.MQTTPassword)
	opts.SetAutoReconnect(true)
	opts.SetConnectRetry(true)
	opts.SetConnectRetryInterval(3 * time.Second)
	opts.SetKeepAlive(30 * time.Second)
	opts.OnConnect = func(_ pahomqtt.Client) {
		c.connected.Store(true)
		c.log.Info("mqtt publish connected", "broker", c.cfg.MQTTBroker)
	}
	opts.OnConnectionLost = func(_ pahomqtt.Client, err error) {
		c.connected.Store(false)
		c.log.Warn("mqtt connection lost", "err", err)
	}
	c.mqtt = pahomqtt.NewClient(opts)
	tok := c.mqtt.Connect()
	ok := tok.WaitTimeout(8 * time.Second)
	if !ok {
		return fmt.Errorf("mqtt connect timeout")
	}
	return tok.Error()
}

func amqpURLFromMQTT(broker, user, pass string) string {
	host := strings.TrimPrefix(broker, "tcp://")
	host = strings.TrimPrefix(host, "ssl://")
	host = strings.TrimPrefix(host, "mqtt://")
	// replace MQTT port with AMQP if default
	if strings.HasSuffix(host, ":1883") {
		host = strings.TrimSuffix(host, ":1883") + ":5672"
	}
	return fmt.Sprintf("amqp://%s:%s@%s/", user, pass, host)
}

func (c *Client) startAMQPIngest() error {
	url := amqpURLFromMQTT(c.cfg.MQTTBroker, c.cfg.MQTTUsername, c.cfg.MQTTPassword)
	conn, err := amqp.DialConfig(url, amqp.Config{
		Dial: amqp.DefaultDial(5 * time.Second),
	})
	if err != nil {
		return fmt.Errorf("amqp dial: %w", err)
	}
	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return err
	}
	q, err := ch.QueueDeclare("crearts.platform.ingest", true, false, false, false, nil)
	if err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return err
	}
	binds := []string{"iot.v1.*.*.up.#", "v1.#"}
	if c.cfg.RulesBusEnabled {
		binds = append(binds, strings.ReplaceAll(c.cfg.RulesBusPrefix, "/", ".")+".commands.*.*.#")
	}
	for _, rk := range binds {
		if err := ch.QueueBind(q.Name, rk, "amq.topic", false, nil); err != nil {
			_ = ch.Close()
			_ = conn.Close()
			return fmt.Errorf("bind %s: %w", rk, err)
		}
	}
	deliveries, err := ch.Consume(q.Name, "crearts-platform", true, false, false, false, nil)
	if err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return err
	}
	c.amqpConn = conn
	c.amqpCh = ch
	c.connected.Store(true)
	go c.consume(deliveries)
	c.log.Info("amqp ingest started", "queue", q.Name)
	return nil
}

func (c *Client) consume(deliveries <-chan amqp.Delivery) {
	for {
		select {
		case <-c.stop:
			return
		case d, ok := <-deliveries:
			if !ok {
				return
			}
			topic := strings.ReplaceAll(d.RoutingKey, ".", "/")
			c.handle(topic, d.Body, d.UserId)
		}
	}
}

func (c *Client) Close() {
	close(c.stop)
	if c.amqpCh != nil {
		_ = c.amqpCh.Close()
	}
	if c.amqpConn != nil {
		_ = c.amqpConn.Close()
	}
	if c.mqtt != nil && c.mqtt.IsConnected() {
		c.mqtt.Disconnect(250)
	}
}

func (c *Client) Publish(topic string, qos byte, retained bool, payload []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.mqtt != nil && c.mqtt.IsConnected() {
		tok := c.mqtt.Publish(topic, qos, retained, payload)
		tok.Wait()
		return tok.Error()
	}
	// fallback AMQP publish to amq.topic
	if c.amqpCh == nil {
		return fmt.Errorf("mqtt not connected")
	}
	rk := strings.ReplaceAll(topic, "/", ".")
	return c.amqpCh.Publish("amq.topic", rk, false, false, amqp.Publishing{
		ContentType: "application/json",
		Body:        payload,
	})
}

func (c *Client) handle(topic string, payload []byte, userID string) {
	recv := db.NowMS()
	if strings.Contains(topic, "/commands/") {
		c.handleCommand(topic, payload)
		return
	}
	dev, capOp, err := c.resolve(topic, userID)
	if err != nil || dev == nil {
		c.log.Debug("mqtt drop", "topic", topic, "user", userID, "err", err)
		return
	}
	switch capOp {
	case "status", "s":
		var st struct {
			Online bool   `json:"online"`
			Reason string `json:"reason"`
		}
		_ = json.Unmarshal(payload, &st)
		if !st.Online {
			reason := st.Reason
			if reason == "" {
				reason = "lwt"
			}
			c.plat.MarkOffline(dev, reason)
		}
	case "telemetry/data", "t":
		_ = c.plat.StoreTelemetry(dev, payload)
	case "events/post", "e":
		_ = c.plat.StoreEvent(dev, payload)
	case "attributes/report", "a":
		var attrs map[string]any
		if err := json.Unmarshal(payload, &attrs); err == nil {
			_ = c.plat.MergeReported(dev, attrs)
		}
	case "attributes/request", "a/req":
		_ = c.plat.ReplyAttributeRequest(dev, payload)
	case "rpc/response", "r/res":
		_ = c.plat.CompleteRPC(dev, payload)
	case "rpc/request", "r/creq":
		_ = c.plat.HandleClientRPC(dev, payload)
	case "ntp/request", "n/req":
		_ = c.plat.ReplyNTP(dev, payload, recv)
	case "ota/version", "o/ver":
		_ = c.plat.HandleOTAVersion(dev, payload)
	case "ota/query", "o/q":
		_ = c.plat.HandleOTAQuery(dev, payload)
	case "ota/progress", "o/p":
		_ = c.plat.HandleOTAProgress(dev, payload)
	case "logs/report", "l":
		_ = c.plat.StoreLogs(dev, payload)
	default:
		c.log.Debug("unhandled capability", "cap", capOp, "topic", topic)
	}
}

func (c *Client) resolve(topic, userID string) (*models.Device, string, error) {
	parts := strings.Split(topic, "/")
	if len(parts) >= 6 && parts[0] == "iot" && parts[1] == "v1" && parts[4] == "up" {
		productID, deviceID := parts[2], parts[3]
		capOp := strings.Join(parts[5:], "/")
		d, err := c.plat.GetDeviceByIdentity(productID, deviceID)
		if err != nil {
			return nil, "", err
		}
		if !d.Enabled {
			return nil, "", fmt.Errorf("disabled")
		}
		if d.Status != "online" {
			c.plat.MarkOnline(d)
		}
		return d, capOp, nil
	}
	if len(parts) >= 2 && parts[0] == "v1" {
		capOp := strings.Join(parts[1:], "/")
		var d *models.Device
		if userID != "" && userID != c.cfg.MQTTUsername {
			// Preferred: username = {product}.{device}
			d = c.plat.ResolveByClientID(userID)
			if d == nil {
				// Legacy: username was the access token
				d = c.plat.ResolveByToken(userID)
			}
		}
		if d == nil {
			return nil, "", fmt.Errorf("cannot resolve short-topic device (no user_id)")
		}
		if !d.Enabled {
			return nil, "", fmt.Errorf("disabled")
		}
		if d.Status != "online" {
			c.plat.MarkOnline(d)
		}
		return d, capOp, nil
	}
	return nil, "", fmt.Errorf("unknown topic shape")
}

func (c *Client) handleCommand(topic string, payload []byte) {
	parts := strings.Split(topic, "/")
	idx := -1
	for i, p := range parts {
		if p == "commands" {
			idx = i
			break
		}
	}
	if idx < 0 || len(parts) < idx+3 {
		return
	}
	productID := parts[idx+1]
	deviceID := parts[idx+2]
	kind := strings.Join(parts[idx+3:], "/")
	d, err := c.plat.GetDeviceByIdentity(productID, deviceID)
	if err != nil {
		c.log.Warn("rules command unknown device", "product", productID, "device", deviceID)
		return
	}
	switch kind {
	case "rpc":
		var req services.RPCRequest
		if err := json.Unmarshal(payload, &req); err != nil {
			return
		}
		_, _ = c.plat.InvokeRPC(d, req, false)
	case "attributes/desired":
		var attrs map[string]any
		if err := json.Unmarshal(payload, &attrs); err != nil {
			return
		}
		_ = c.plat.SetDesired(d, attrs, true)
	case "ota":
		var body struct {
			FirmwareID int64 `json:"firmware_id"`
		}
		if err := json.Unmarshal(payload, &body); err != nil {
			return
		}
		_, _ = c.plat.StartOTAJob(d.ID, body.FirmwareID)
	}
}
