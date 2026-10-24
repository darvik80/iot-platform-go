package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"cogitor-iot-platform/internal/models"
)

func apiMonitorWidget(device int64) models.MonitoringWidget {
	return models.MonitoringWidget{ID: "w1", Title: "Temperature", Type: "line", WindowSeconds: 3600, Aggregation: "avg", Width: 6, Sources: []models.MetricSource{{DeviceID: device, Key: "temperature", Color: "#123456"}}}
}

func TestMonitoringAPIDashboardsAndTenantIsolation(t *testing.T) {
	e := newTestEnv(t)
	e.user("monitorowner")
	e.user("monitorother")
	d := e.device("monitorowner", "monitor", "sensor")
	base := "/api/v1/monitoring"
	e.expect(e.do("GET", base+"/dashboards", "", nil), 401, "anonymous")
	in := models.MonitoringDashboard{Name: "Factory", Widgets: []models.MonitoringWidget{apiMonitorWidget(d.ID)}}
	rec := e.do("POST", base+"/dashboards", "monitorowner", in)
	e.expect(rec, 201, "create")
	dash := decode[models.MonitoringDashboard](t, rec)
	path := base + "/dashboards/" + itoa(dash.ID)
	e.expect(e.do("GET", path, "monitorowner", nil), 200, "load")
	for _, method := range []string{"GET", "PUT", "DELETE"} {
		e.expect(e.do(method, path, "monitorother", models.MonitoringDashboard{Name: "stolen", Version: 1}), 404, "foreign "+method)
	}
	list := decode[[]models.MonitoringDashboard](t, e.do("GET", base+"/dashboards", "monitorother", nil))
	if len(list) != 0 {
		t.Fatalf("foreign list: %+v", list)
	}
	e.expect(e.do("POST", base+"/dashboards", "monitorother", in), 404, "foreign widget")
	e.expect(e.do("GET", base+"/metrics?device_id="+itoa(d.ID), "monitorother", nil), 404, "foreign metrics")
	e.expect(e.do("POST", base+"/query", "monitorother", models.MonitoringQuery{Widget: in.Widgets[0]}), 404, "foreign query")
	if err := e.plat.StoreTelemetry(d, []byte(`{"temperature":23.5}`)); err != nil {
		t.Fatal(err)
	}
	metrics := decode[[]models.MonitoringMetric](t, e.do("GET", base+"/metrics?device_id="+itoa(d.ID), "monitorowner", nil))
	if len(metrics) != 1 || metrics[0].Key != "temperature" {
		t.Fatalf("metrics: %+v", metrics)
	}
	data := decode[models.MonitoringData](t, e.do("POST", base+"/query", "monitorowner", models.MonitoringQuery{Widget: in.Widgets[0]}))
	if len(data.Series) != 1 || len(data.Series[0].Points) != 1 || data.Series[0].Points[0].Value != 23.5 {
		t.Fatalf("query: %+v", data)
	}
	dash.Name = "Renamed"
	e.expect(e.do("PUT", path, "monitorowner", dash), 200, "update")
	e.expect(e.do("PUT", path, "monitorowner", dash), 409, "stale version")
	e.expect(e.do("DELETE", path, "monitorowner", nil), 204, "delete")
	e.expect(e.do("GET", path, "monitorowner", nil), 404, "deleted")
}

func TestMonitoringAPICurrentRolesAndSuspension(t *testing.T) {
	e := newTestEnv(t)
	u := e.user("monitorroles")
	account := e.accountID("monitorroles")
	d := e.device("monitorroles", "monitor", "sensor")
	base := "/api/v1/monitoring"
	dash := models.MonitoringDashboard{Name: "Roles", Widgets: []models.MonitoringWidget{apiMonitorWidget(d.ID)}}
	for _, role := range []string{"viewer", "operator", "admin"} {
		t.Run(role, func(t *testing.T) {
			if _, err := e.plat.DB.Exec(`UPDATE account_memberships SET role=? WHERE user_id=? AND account_id=?`, role, u.ID, account); err != nil {
				t.Fatal(err)
			}
			e.expect(e.do("GET", base+"/dashboards", "monitorroles", nil), 200, "read")
			e.expect(e.do("POST", base+"/query", "monitorroles", models.MonitoringQuery{Widget: dash.Widgets[0]}), 200, "read query")
			write := 201
			if role == "viewer" {
				write = 403
			}
			e.expect(e.do("POST", base+"/dashboards", "monitorroles", dash), write, "dashboard write")
			admin := 200
			if role != "admin" {
				admin = 403
			}
			e.expect(e.do("GET", base+"/channels", "monitorroles", nil), admin, "channel access")
			for _, path := range []string{"/rules", "/channels"} {
				status := 400
				if role != "admin" {
					status = 403
				}
				e.expect(e.do("POST", base+path, "monitorroles", map[string]any{}), status, "admin writes")
			}
		})
	}
	if _, err := e.plat.DB.Exec(`UPDATE accounts SET status='suspended' WHERE id=?`, account); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/dashboards", "/rules", "/channels", "/events", "/deliveries"} {
		e.expect(e.do("GET", base+path, "monitorroles", nil), 403, "suspended")
	}
	e.expect(e.do("POST", base+"/dashboards", "monitorroles", dash), 403, "suspended write")
}

func TestMonitoringAPIChannelsRulesAndHistoryIsolation(t *testing.T) {
	e := newTestEnv(t)
	e.user("alertowner")
	e.user("alertother")
	d := e.device("alertowner", "monitor", "sensor")
	base := "/api/v1/monitoring"
	input := models.NotificationChannelInput{NotificationChannel: models.NotificationChannel{Name: "Ops", Kind: "telegram", Destination: "-12345", Enabled: true}, Secret: "123456:ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijk"}
	rec := e.do("POST", base+"/channels", "alertowner", input)
	e.expect(rec, 201, "channel create")
	channel := decode[models.NotificationChannel](t, rec)
	if !channel.HasSecret || strings.Contains(rec.Body.String(), input.Secret) {
		t.Fatal("secret masking")
	}
	path := base + "/channels/" + itoa(channel.ID)
	for _, method := range []string{"PUT", "DELETE"} {
		e.expect(e.do(method, path, "alertother", input), 404, "foreign channel "+method)
	}
	e.expect(e.do("POST", path+"/test", "alertother", nil), 404, "foreign channel test")
	input.Secret = ""
	input.Name = "Operations"
	e.expect(e.do("PUT", path, "alertowner", input), 200, "preserve secret")
	rule := models.AlertRule{Name: "Hot", DeviceID: d.ID, Metric: "temperature", Kind: "threshold", Operator: "gt", Threshold: 30, Recovery: 28, MissingSeconds: 300, CooldownSeconds: 60, Enabled: true, ChannelIDs: []int64{channel.ID}}
	rec = e.do("POST", base+"/rules", "alertowner", rule)
	e.expect(rec, 201, "rule create")
	created := decode[models.AlertRule](t, rec)
	rulePath := base + "/rules/" + itoa(created.ID)
	e.expect(e.do("POST", base+"/rules", "alertother", rule), 400, "foreign rule source")
	for _, method := range []string{"PUT", "DELETE"} {
		e.expect(e.do(method, rulePath, "alertother", rule), 404, "foreign rule "+method)
	}
	otherDevice := e.device("alertother", "other-monitor", "sensor")
	rule.DeviceID = otherDevice.ID
	e.expect(e.do("POST", base+"/rules", "alertother", rule), 400, "foreign rule channel")
	for _, resource := range []string{"channels", "rules", "events", "deliveries"} {
		rec := e.do("GET", base+"/"+resource, "alertother", nil)
		e.expect(rec, 200, "foreign list")
		if strings.TrimSpace(rec.Body.String()) != "[]" {
			t.Fatalf("%s isolation: %s", resource, rec.Body.String())
		}
	}
	e.expect(e.do("DELETE", rulePath, "alertowner", nil), 204, "delete rule")
	e.expect(e.do("DELETE", path, "alertowner", nil), 204, "delete channel")
}

func TestMonitoringAPIBoundaryValidation(t *testing.T) {
	e := newTestEnv(t)
	e.user("monitorinvalid")
	base := "/api/v1/monitoring"
	for _, path := range []string{"/dashboards/0", "/dashboards/-1", "/dashboards/nope", "/metrics", "/metrics?device_id=0", "/events?limit=-1", "/deliveries?limit=501", "/events?limit=abc"} {
		e.expect(e.do("GET", base+path, "monitorinvalid", nil), 400, path)
	}
	for _, body := range []string{`{`, `{"name":"x","unknown":true}`, `{"name":"x"} {}`, `null`, `[]`, `{"name":"` + strings.Repeat("x", 128<<10) + `"}`} {
		req := httptest.NewRequest(http.MethodPost, base+"/dashboards", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+e.tokens["monitorinvalid"])
		rec := httptest.NewRecorder()
		e.h.ServeHTTP(rec, req)
		if rec.Code != 400 && rec.Code != 413 {
			t.Fatalf("invalid body accepted: status=%d", rec.Code)
		}
	}
}
