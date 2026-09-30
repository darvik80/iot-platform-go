package services

import (
	"testing"

	"cogitor-iot-platform/internal/config"
)

func TestDeviceProvisionJSON(t *testing.T) {
	p := newTestPlatform(t, config.Config{
		Platform: config.PlatformConfig{BrokerHostPublic: "192.168.1.100"},
		MQTT:     config.MQTTConfig{Addr: ":1883", WSAddr: ":8083"},
	})
	if _, err := p.CreateProduct(1, ProductInput{ProductID: "home", Name: "Home"}); err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	_, creds, err := p.CreateDevice(DeviceInput{ProductID: "home", DeviceID: "esp32-s3"})
	if err != nil {
		t.Fatalf("CreateDevice: %v", err)
	}
	if creds.Provision == nil {
		t.Fatal("missing provision")
	}
	c := creds.Provision.Cogitor
	if c.Product != "home" || c.Device != "esp32-s3" || c.Host != "192.168.1.100" || c.Port != 1883 {
		t.Fatalf("cogitor=%+v", c)
	}
	if c.WSURL != "ws://192.168.1.100:8083/" || c.WSPort != 8083 {
		t.Fatalf("ws=%+v", c)
	}
	if c.Token == "" || c.TLS || !c.TopicShort {
		t.Fatalf("token/tls/topic_short=%+v", c)
	}
}
