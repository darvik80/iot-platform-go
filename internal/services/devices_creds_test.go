package services

import (
	"path/filepath"
	"testing"

	"crearts-iot-platform/internal/config"
	"crearts-iot-platform/internal/db"
)

func TestDeviceProvisionJSON(t *testing.T) {
	dir := t.TempDir()
	database, err := db.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	p := New(database, config.Config{
		SQLitePath:       filepath.Join(dir, "test.db"),
		BrokerHostPublic: "192.168.1.100",
		MQTTAddr:         ":1883",
	}, nil)
	if _, err := p.CreateProduct(ProductInput{ProductID: "home", Name: "Home"}); err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	_, creds, err := p.CreateDevice(DeviceInput{ProductID: "home", DeviceID: "esp32-s3"})
	if err != nil {
		t.Fatalf("CreateDevice: %v", err)
	}
	if creds.Provision == nil {
		t.Fatal("missing provision")
	}
	c := creds.Provision.Crearts
	if c.Product != "home" || c.Device != "esp32-s3" || c.Host != "192.168.1.100" || c.Port != 1883 {
		t.Fatalf("crearts=%+v", c)
	}
	if c.Token == "" || c.TLS || !c.TopicShort {
		t.Fatalf("token/tls/topic_short=%+v", c)
	}
}
