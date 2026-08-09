package mqtt

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"
)

func normalizeMQTTAddr(addr string) string {
	addr = strings.TrimSpace(addr)
	addr = strings.TrimPrefix(addr, "tcp://")
	addr = strings.TrimPrefix(addr, "mqtt://")
	if addr == "" {
		addr = ":1883"
	}
	if !strings.Contains(addr, ":") {
		addr = ":" + addr
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	// ESP32 is IPv4. Empty / IPv6-any host on Windows often shares the port
	// with another broker via SO_REUSEADDR; bind IPv4 explicitly.
	if host == "" || host == "*" {
		host = "0.0.0.0"
	}
	return net.JoinHostPort(host, port)
}

func listenTCP(addr string) (net.Listener, error) {
	addr = normalizeMQTTAddr(addr)
	lc := net.ListenConfig{
		KeepAlive: 30 * time.Second,
		Control:   exclusiveControl,
	}
	ln, err := lc.Listen(context.Background(), "tcp4", addr)
	if err != nil {
		return nil, fmt.Errorf("%s: %w — another MQTT broker is using this port (RabbitMQ/Mosquitto/Windows). Stop it or set MQTT_ADDR", addr, err)
	}
	return ln, nil
}
