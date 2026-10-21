package api

import (
	"context"
	"net/http"
	"testing"

	"cogitor-iot-platform/internal/mcpbridge"
	"cogitor-iot-platform/internal/models"
	"cogitor-iot-platform/internal/plugins"
	"cogitor-iot-platform/internal/services"
)

func TestCreateDeviceStaysInOwnAccount(t *testing.T) {
	e := newTestEnv(t)
	e.user("alice")
	e.user("bob")
	e.device("alice", "lamp", "seed")
	if _, err := e.plat.CreateProduct(e.accountID("bob"), services.ProductInput{ProductID: "home", Name: "Home"}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.plat.CreateProduct(e.accountID("alice"), services.ProductInput{ProductID: "home", Name: "Home"}); err != nil {
		t.Fatal(err)
	}

	e.expect(e.do("POST", "/api/v1/devices", "bob", map[string]any{"product_id": "lamp", "device_id": "evil"}),
		http.StatusBadRequest, "bob creates a device under alice's product")

	rec := e.do("POST", "/api/v1/devices", "bob", map[string]any{"product_id": "home", "device_id": "d1"})
	e.expect(rec, http.StatusCreated, "bob creates a device under his own product")
	out := decode[struct {
		Device models.Device `json:"device"`
	}](t, rec)
	if out.Device.AccountID != e.accountID("bob") {
		t.Errorf("device landed in account %d, want bob's %d", out.Device.AccountID, e.accountID("bob"))
	}

	e.expect(e.do("POST", "/api/v1/devices", "alice", map[string]any{"product_id": "home", "device_id": "d1"}),
		http.StatusBadRequest, "duplicate MQTT identity home.d1 in another account")
}

func TestFirmwareScopedToAccount(t *testing.T) {
	e := newTestEnv(t)
	e.user("alice")
	e.user("bob")
	e.device("alice", "lamp", "l1")
	fw, err := e.plat.RegisterFirmware(e.accountID("alice"), services.FirmwareInput{
		ProductID: "lamp", Version: "1.0.0", URL: "http://example.com/fw.bin",
	})
	if err != nil {
		t.Fatal(err)
	}

	e.expect(e.do("POST", "/api/v1/ota/firmwares", "bob", map[string]any{
		"product_id": "lamp", "version": "6.6.6", "url": "http://evil/fw.bin",
	}), http.StatusBadRequest, "bob registers firmware for alice's product")
	e.expect(e.do("POST", "/api/v1/ota/firmwares/"+itoa(fw.ID)+"/notify", "bob", map[string]any{}),
		http.StatusNotFound, "bob pushes alice's firmware")
	e.expect(e.do("DELETE", "/api/v1/ota/firmwares/"+itoa(fw.ID), "bob", nil),
		http.StatusNotFound, "bob deletes alice's firmware")
	e.expect(e.do("DELETE", "/api/v1/ota/firmwares/"+itoa(fw.ID), "alice", nil),
		http.StatusNoContent, "alice deletes her firmware")
}

func TestPluginBindingsScopedToAccount(t *testing.T) {
	e := newTestEnv(t)
	e.user("alice")
	e.user("bob")
	aliceDev := e.device("alice", "lamp", "l1")
	bobDev := e.device("bob", "home", "h1")
	b, err := e.plat.BindDevicePlugin(aliceDev.ID, services.PluginBindingInput{PluginID: plugins.PluginLEDGarland})
	if err != nil {
		t.Fatal(err)
	}

	e.expect(e.do("GET", "/api/v1/devices/"+itoa(aliceDev.ID)+"/plugins", "bob", nil),
		http.StatusNotFound, "bob lists alice's bindings")
	e.expect(e.do("POST", "/api/v1/devices/"+itoa(aliceDev.ID)+"/plugins", "bob",
		map[string]any{"plugin_id": plugins.PluginLEDGarland}), http.StatusNotFound, "bob binds to alice's device")
	// Own device in the path, foreign binding id.
	path := "/api/v1/devices/" + itoa(bobDev.ID) + "/plugins/" + itoa(b.ID)
	e.expect(e.do("PUT", path, "bob", map[string]any{"enabled": false}), http.StatusNotFound, "bob updates alice's binding")
	e.expect(e.do("DELETE", path, "bob", nil), http.StatusNotFound, "bob deletes alice's binding")
	e.expect(e.do("POST", path+"/apply", "bob", nil), http.StatusNotFound, "bob applies alice's binding")

	e.expect(e.do("DELETE", "/api/v1/devices/"+itoa(aliceDev.ID)+"/plugins/"+itoa(b.ID), "alice", nil),
		http.StatusNoContent, "alice deletes her binding")
}

func TestExternalPluginsScopedToAccount(t *testing.T) {
	e := newTestEnv(t)
	e.user("alice")
	e.user("bob")

	e.expect(e.do("POST", "/api/v1/plugins/external", "alice", map[string]any{"id": "bot", "name": "Bot"}),
		http.StatusCreated, "alice registers bot")
	e.expect(e.do("POST", "/api/v1/plugins/external", "bob", map[string]any{"id": "bad/id", "name": "X"}),
		http.StatusBadRequest, "invalid plugin id")
	e.expect(e.do("POST", "/api/v1/plugins/external", "bob", map[string]any{"id": "bot", "name": "Mine now"}),
		http.StatusBadRequest, "bob takes over alice's plugin id")

	list := e.do("GET", "/api/v1/plugins/external", "bob", nil)
	e.expect(list, http.StatusOK, "bob lists plugins")
	if got := decode[[]models.ExternalPlugin](t, list); len(got) != 0 {
		t.Errorf("bob sees %d external plugins, want 0", len(got))
	}
	e.expect(e.do("POST", "/api/v1/plugins/external/bot/regenerate-mqtt", "bob", nil),
		http.StatusNotFound, "bob regenerates alice's plugin password")
	e.expect(e.do("DELETE", "/api/v1/plugins/external/bot", "bob", nil),
		http.StatusNotFound, "bob deletes alice's plugin")

	if p, ok := e.plat.ExternalPlugins.GetPlugin("bot"); !ok || p.AccountID != e.accountID("alice") {
		t.Fatalf("alice's plugin was modified: %+v", p)
	}
	e.expect(e.do("POST", "/api/v1/plugins/external/bot/regenerate-mqtt", "alice", nil),
		http.StatusOK, "alice regenerates her plugin password")
}

type fakeMQTT struct{ st models.MQTTStatus }

func (f fakeMQTT) DropDevice(int64)          {}
func (f fakeMQTT) Status() models.MQTTStatus { return f.st }

func TestMQTTStatusScopedToAccount(t *testing.T) {
	e := newTestEnv(t)
	alice := e.user("alice")
	e.user("bob")
	e.makePlatformAdmin(alice)
	bobDev := e.device("bob", "home", "h1")
	e.device("alice", "lamp", "l1")
	e.plat.SetMQTTSession(fakeMQTT{st: models.MQTTStatus{
		Clients: []models.MQTTClient{
			{Username: "home.h1"}, {Username: "lamp.l1"}, {Username: "extplg_bob_bot"}, {Username: "extplg_alice_bot"},
		},
		Events: []models.MQTTEvent{{Username: "home.h1"}, {Username: "lamp.l1"}, {Username: "stranger"}},
	}})

	rec := e.do("GET", "/api/v1/mqtt", "bob", nil)
	e.expect(rec, http.StatusOK, "bob mqtt status")
	st := decode[models.MQTTStatus](t, rec)
	if len(st.Clients) != 2 || st.ClientCount != 2 {
		t.Errorf("bob sees clients %+v, want home.h1 and extplg_bob_bot", st.Clients)
	}
	for _, c := range st.Clients {
		if c.Username != bobDev.ProductID+"."+bobDev.DeviceID && c.Username != "extplg_bob_bot" {
			t.Errorf("bob sees foreign client %q", c.Username)
		}
	}
	if len(st.Events) != 1 || st.Events[0].Username != "home.h1" {
		t.Errorf("bob sees events %+v, want only home.h1", st.Events)
	}

	all := decode[models.MQTTStatus](t, e.do("GET", "/api/v1/mqtt", "alice", nil))
	if len(all.Clients) != 4 || len(all.Events) != 3 {
		t.Errorf("platform admin sees %d clients / %d events, want 4 / 3", len(all.Clients), len(all.Events))
	}
}

func TestMCPLocalScopedToAccount(t *testing.T) {
	e := newTestEnv(t)
	e.user("alice")
	e.user("bob")
	aliceDev := e.device("alice", "lamp", "l1")
	bobDev := e.device("bob", "home", "h1")
	ctx := context.Background()

	viewer := mcpbridge.NewLocal(e.plat, models.RoleViewer, e.accountID("bob"), "bob")
	if _, err := viewer.GetDevice(ctx, aliceDev.ID); err == nil {
		t.Error("bob's MCP reads alice's device")
	}
	if _, err := viewer.FindDeviceID(ctx, "lamp", "l1"); err == nil {
		t.Error("bob's MCP resolves alice's device identity")
	}
	if _, err := viewer.GetAttributes(ctx, aliceDev.ID); err == nil {
		t.Error("bob's MCP reads alice's attributes")
	}
	if _, err := viewer.SetDesired(ctx, bobDev.ID, map[string]any{"x": 1}); err == nil {
		t.Error("viewer role writes desired attributes")
	}
	list, err := viewer.ListDevices(ctx, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if devs := list.([]models.Device); len(devs) != 1 || devs[0].ID != bobDev.ID {
		t.Errorf("bob's MCP lists %+v, want only his device", devs)
	}

	noRole := mcpbridge.NewLocal(e.plat, "", e.accountID("bob"), "bob")
	if _, err := noRole.SetDesired(ctx, bobDev.ID, map[string]any{"x": 1}); err == nil {
		t.Error("empty role bypasses the write check")
	}
}

func TestMCPEndpointRequiresMembership(t *testing.T) {
	e := newTestEnv(t)
	e.user("bob")
	if _, err := e.plat.DB.Exec(`UPDATE accounts SET status='suspended' WHERE slug='bob'`); err != nil {
		t.Fatal(err)
	}
	e.expect(e.do("POST", "/mcp", "bob", map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/list"}),
		http.StatusForbidden, "MCP on a suspended account")
}
