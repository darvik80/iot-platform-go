package mqtt

import (
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"cogitor-iot-platform/internal/config"
	"cogitor-iot-platform/internal/db"
	"cogitor-iot-platform/internal/models"
	"cogitor-iot-platform/internal/protocol"
	"cogitor-iot-platform/internal/services"
)

// newIsolationBroker builds a broker (not started) over a platform with two
// accounts, alice and bob, each owning one device.
func newIsolationBroker(t *testing.T) (*Broker, *services.Platform, *models.Device, *models.Device) {
	t.Helper()
	dir := t.TempDir()
	database, err := db.Open("sqlite", filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })

	var cfg config.Config
	cfg.DataDir = dir
	cfg.Rules.Prefix = "platform/v1"
	cfg.Rules.Enabled = false
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	plat := services.New(database, cfg, log)

	mk := func(slug, product, device string) *models.Device {
		now := db.NowMS()
		res, err := database.Exec(`INSERT INTO accounts(slug, name, status, created_at, updated_at) VALUES(?,?, 'active', ?, ?)`, slug, slug, now, now)
		if err != nil {
			t.Fatal(err)
		}
		acc, _ := res.LastInsertId()
		if _, err := plat.CreateProduct(acc, services.ProductInput{ProductID: product, Name: product}); err != nil {
			t.Fatal(err)
		}
		d, _, err := plat.CreateDevice(acc, services.DeviceInput{ProductID: product, DeviceID: device})
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	alice := mk("alice", "lamp", "l1")
	bob := mk("bob", "home", "h1")
	return New(cfg, plat, log), plat, alice, bob
}

func desiredOf(t *testing.T, plat *services.Platform, d *models.Device) map[string]any {
	t.Helper()
	_, desired, err := plat.GetAttributes(d.ID)
	if err != nil {
		t.Fatal(err)
	}
	return desired
}

func TestPluginCommandsStayInOwnAccount(t *testing.T) {
	b, plat, alice, bob := newIsolationBroker(t)
	payload := []byte(`{"color":"red"}`)

	b.handleCommand("platform/v1/commands/alice/lamp/l1/attributes/desired", payload, "extplg_bob_bot")
	b.handleCommand("platform/v1/commands/bob/lamp/l1/attributes/desired", payload, "extplg_bob_bot")
	b.handleCommand("platform/v1/commands/lamp/l1/attributes/desired", payload, "extplg_bob_bot")
	if got := desiredOf(t, plat, alice); len(got) != 0 {
		t.Errorf("bob's plugin changed alice's device: %v", got)
	}

	b.handleCommand("platform/v1/commands/bob/home/h1/attributes/desired", payload, "extplg_bob_bot")
	if got := desiredOf(t, plat, bob); got["color"] != "red" {
		t.Errorf("own-account command not applied, desired=%v", got)
	}
}

func TestFullStyleUplinkBoundToSession(t *testing.T) {
	b, _, alice, bob := newIsolationBroker(t)
	bobUser := bob.ProductID + "." + bob.DeviceID

	foreign := protocol.FullUpRoot(alice.ProductID, alice.DeviceID) + protocol.UpTelemetry
	if _, _, err := b.resolve(foreign, bobUser); err == nil {
		t.Error("bob's session published as alice's device")
	}
	own := protocol.FullUpRoot(bob.ProductID, bob.DeviceID) + protocol.UpTelemetry
	d, _, err := b.resolve(own, bobUser)
	if err != nil || d.ID != bob.ID {
		t.Errorf("own full-style uplink: device=%v err=%v", d, err)
	}
}
