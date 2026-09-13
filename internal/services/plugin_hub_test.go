package services

import (
	"context"
	"path/filepath"
	"testing"

	"cogitor-iot-platform/internal/config"
	"cogitor-iot-platform/internal/db"
	"cogitor-iot-platform/internal/plugins"
)

func TestBindLEDGarlandPlugin(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	database, err := db.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	p := New(database, config.Config{
		DatabaseDriver: "sqlite",
		DatabaseDSN:    dbPath,
		DataDir:        dir,
	}, nil)
	if _, err := p.CreateProduct(ProductInput{ProductID: "home", Name: "Home"}); err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	dev, _, err := p.CreateDevice(DeviceInput{ProductID: "home", DeviceID: "esp32-s3-ctr"})
	if err != nil {
		t.Fatalf("CreateDevice: %v", err)
	}

	cat := p.ListPluginCatalog()
	foundLED, foundSched := false, false
	for _, c := range cat {
		if c.ID == plugins.PluginLEDGarland {
			foundLED = true
		}
		if c.ID == plugins.PluginSchedule {
			foundSched = true
		}
	}
	if !foundLED {
		t.Fatal("led-garland missing from catalog")
	}
	if !foundSched {
		t.Fatal("schedule missing from catalog")
	}
	for _, c := range cat {
		if c.ID == plugins.PluginLEDGarland && len(c.Form) < 10 {
			t.Fatal("led-garland form schema missing")
		}
		if c.ID == plugins.PluginSchedule && len(c.Form) < 10 {
			t.Fatal("schedule form schema missing")
		}
	}

	b, err := p.BindDevicePlugin(dev.ID, PluginBindingInput{PluginID: plugins.PluginLEDGarland})
	if err != nil {
		t.Fatalf("BindDevicePlugin: %v", err)
	}
	if b.PluginID != plugins.PluginLEDGarland || !b.Enabled || len(b.Config) == 0 {
		t.Fatalf("binding=%+v", b)
	}

	if _, err := p.BindDevicePlugin(dev.ID, PluginBindingInput{PluginID: plugins.PluginLEDGarland}); err == nil {
		t.Fatal("expected duplicate bind error")
	}

	en := false
	updated, err := p.UpdatePluginBinding(b.ID, PluginBindingInput{Enabled: &en})
	if err != nil {
		t.Fatalf("UpdatePluginBinding: %v", err)
	}
	if updated.Enabled {
		t.Fatal("expected disabled")
	}

	list, err := p.ListDevicePlugins(dev.ID)
	if err != nil || len(list) != 1 {
		t.Fatalf("list=%v err=%v", list, err)
	}

	if err := p.DeletePluginBinding(b.ID); err != nil {
		t.Fatalf("DeletePluginBinding: %v", err)
	}
	list, err = p.ListDevicePlugins(dev.ID)
	if err != nil || len(list) != 0 {
		t.Fatalf("list after delete=%v err=%v", list, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p.StartPlugins(ctx)
}
