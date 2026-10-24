package services

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/url"
	"os"
	"reflect"
	"strings"
	"testing"

	"cogitor-iot-platform/internal/config"
	"cogitor-iot-platform/internal/db"
	"cogitor-iot-platform/internal/models"
)

func newDashboardTestPlatform(t *testing.T) *Platform {
	t.Helper()
	dsn := os.Getenv("MONITOR_TEST_POSTGRES_DSN")
	if dsn == "" {
		return newTestPlatform(t, config.Config{})
	}
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = raw.Close() })
	schema := "monitor_test_" + strings.ToLower(rand.Text())
	if _, err := raw.Exec(`CREATE SCHEMA "` + schema + `"`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := raw.Exec(`DROP SCHEMA "` + schema + `" CASCADE`); err != nil {
			t.Error(err)
		}
	})
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	database, err := db.Open("postgres", u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if _, err := database.Exec(`INSERT INTO accounts(id,slug,name,status,created_at,updated_at) VALUES(1,'default','Default','active',?,?)`, db.NowMS(), db.NowMS()); err != nil {
		t.Fatal(err)
	}
	return New(database, config.Config{DataDir: t.TempDir(), Auth: config.AuthConfig{JWTSecret: "monitoring-integration-secret"}}, slog.New(slog.DiscardHandler))
}

func dashboardDevice(t *testing.T, p *Platform, account int64) *models.Device {
	t.Helper()
	product := fmt.Sprintf("monitor-%d", account)
	if _, err := p.CreateProduct(account, ProductInput{ProductID: product, Name: product}); err != nil {
		t.Fatal(err)
	}
	d, _, err := p.CreateDevice(account, DeviceInput{ProductID: product, DeviceID: "sensor"})
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func dashboardWidget(device int64) models.MonitoringWidget {
	return models.MonitoringWidget{ID: "temperature", Title: "Temperature", Type: "line", Aggregation: "avg", WindowSeconds: 3600, Width: 6, Sources: []models.MetricSource{{DeviceID: device, Key: "temperature", Color: "#123abc"}}}
}

func TestMonitoringDashboardPersistenceIsolationAndConflict(t *testing.T) {
	p := newDashboardTestPlatform(t)
	d := dashboardDevice(t, p, 1)
	w := dashboardWidget(d.ID)
	in := models.MonitoringDashboard{Name: " Factory ", Widgets: []models.MonitoringWidget{w}}
	created, err := p.SaveMonitoringDashboard(1, 0, in)
	if err != nil {
		t.Fatal(err)
	}
	if created.Version != 1 || created.AccountID != 1 || created.Name != "Factory" {
		t.Fatalf("created: %+v", created)
	}
	restarted := New(p.DB, p.Cfg, p.Log)
	got, err := restarted.GetMonitoringDashboard(1, created.ID)
	if err != nil || !reflect.DeepEqual(got, created) {
		t.Fatalf("reload: %+v, %v", got, err)
	}
	if _, err := p.GetMonitoringDashboard(2, created.ID); !errors.Is(err, ErrMonitorNotFound) {
		t.Fatalf("cross-account get: %v", err)
	}
	if _, err := p.SaveMonitoringDashboard(2, created.ID, models.MonitoringDashboard{Name: "stolen", Version: 1}); !errors.Is(err, ErrMonitorNotFound) {
		t.Fatalf("cross-account update: %v", err)
	}
	if err := p.DeleteMonitoringDashboard(2, created.ID); !errors.Is(err, ErrMonitorNotFound) {
		t.Fatalf("cross-account delete: %v", err)
	}
	list, err := p.ListMonitoringDashboards(2)
	if err != nil || len(list) != 0 {
		t.Fatalf("cross-account list: %+v, %v", list, err)
	}
	stale := *created
	w2 := w
	w2.ID = "second"
	created.Widgets = []models.MonitoringWidget{w2, w}
	created.Name = "Renamed"
	updated, err := p.SaveMonitoringDashboard(1, created.ID, *created)
	if err != nil || updated.Version != 2 {
		t.Fatalf("update: %+v, %v", updated, err)
	}
	if _, err := p.SaveMonitoringDashboard(1, created.ID, stale); !errors.Is(err, ErrMonitorConflict) {
		t.Fatalf("stale write: %v", err)
	}
	got, err = p.GetMonitoringDashboard(1, created.ID)
	if err != nil || !reflect.DeepEqual(got, updated) {
		t.Fatalf("widget order: %+v, %v", got, err)
	}
	if err := p.DeleteMonitoringDashboard(1, created.ID); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := p.DB.QueryRow(`SELECT COUNT(*) FROM monitoring_widgets`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("cascade: %d, %v", count, err)
	}
	if err := p.DeleteMonitoringDashboard(1, created.ID); !errors.Is(err, ErrMonitorNotFound) {
		t.Fatalf("second delete: %v", err)
	}
}

func TestMonitoringWidgetValidation(t *testing.T) {
	p := newDashboardTestPlatform(t)
	d := dashboardDevice(t, p, 1)
	tests := map[string]func(*models.MonitoringWidget){
		"id":                func(w *models.MonitoringWidget) { w.ID = "" },
		"title":             func(w *models.MonitoringWidget) { w.Title = "  " },
		"type":              func(w *models.MonitoringWidget) { w.Type = "script" },
		"sql":               func(w *models.MonitoringWidget) { w.Aggregation = "AVG(value_num)); DROP TABLE devices; --" },
		"window short":      func(w *models.MonitoringWidget) { w.WindowSeconds = 59 },
		"window long":       func(w *models.MonitoringWidget) { w.WindowSeconds = monitorMaxWindow + 1 },
		"width":             func(w *models.MonitoringWidget) { w.Width = 3 },
		"gauge":             func(w *models.MonitoringWidget) { w.Type = "gauge"; w.Min = 100; w.Max = 10 },
		"nan":               func(w *models.MonitoringWidget) { w.Max = math.NaN() },
		"infinity":          func(w *models.MonitoringWidget) { w.Min = math.Inf(1) },
		"empty sources":     func(w *models.MonitoringWidget) { w.Sources = nil },
		"duplicate sources": func(w *models.MonitoringWidget) { w.Sources = append(w.Sources, w.Sources[0]) },
		"empty key":         func(w *models.MonitoringWidget) { w.Sources[0].Key = "" },
		"color":             func(w *models.MonitoringWidget) { w.Sources[0].Color = "url(javascript:bad)" },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			w := dashboardWidget(d.ID)
			mutate(&w)
			if _, err := p.SaveMonitoringDashboard(1, 0, models.MonitoringDashboard{Name: "test", Widgets: []models.MonitoringWidget{w}}); !errors.Is(err, ErrMonitorInvalid) {
				t.Fatalf("save: %v", err)
			}
			if _, err := p.QueryMonitoring(1, models.MonitoringQuery{Widget: w}); !errors.Is(err, ErrMonitorInvalid) {
				t.Fatalf("query: %v", err)
			}
		})
	}
	w := dashboardWidget(d.ID)
	if _, err := p.QueryMonitoring(2, models.MonitoringQuery{Widget: w}); !errors.Is(err, ErrMonitorNotFound) {
		t.Fatalf("foreign source: %v", err)
	}
	if _, err := p.SaveMonitoringDashboard(1, 0, models.MonitoringDashboard{Name: "duplicates", Widgets: []models.MonitoringWidget{w, w}}); !errors.Is(err, ErrMonitorInvalid) {
		t.Fatalf("duplicate ids: %v", err)
	}
	for i := 0; i < 50; i++ {
		if _, err := p.SaveMonitoringDashboard(1, 0, models.MonitoringDashboard{Name: "empty"}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := p.SaveMonitoringDashboard(1, 0, models.MonitoringDashboard{Name: "overflow"}); !errors.Is(err, ErrMonitorInvalid) {
		t.Fatalf("quota: %v", err)
	}
}

func TestMonitoringAggregationCoversEntireWindow(t *testing.T) {
	p := newDashboardTestPlatform(t)
	d := dashboardDevice(t, p, 1)
	from := db.NowMS() - 120000
	tx, err := p.DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	for i := 0; i <= 12000; i++ {
		if _, err := tx.Exec(`INSERT INTO telemetry_samples(device_pk,account_id,key,value_num,ts,ingested_at) VALUES(?,1,'temperature',?,?,?)`, d.ID, float64(i), from+int64(i)*5, from+int64(i)*5); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	w := dashboardWidget(d.ID)
	for _, aggregate := range []string{"avg", "min", "max", "sum", "last"} {
		t.Run(aggregate, func(t *testing.T) {
			w.Aggregation = aggregate
			got, err := p.QueryMonitoring(1, models.MonitoringQuery{Widget: w, From: from, To: from + 60000})
			if err != nil {
				t.Fatal(err)
			}
			points := got.Series[0].Points
			if len(points) != monitorMaxPoints {
				t.Fatalf("points=%d", len(points))
			}
			if points[len(points)-1].TS != from+60000 {
				t.Fatal("last data truncated")
			}
			bucket := int64(60000/monitorMaxPoints + 1)
			for b, point := range points {
				values := []float64{}
				for i := 0; i <= 12000; i++ {
					if int64(i)*5/bucket == int64(b) {
						values = append(values, float64(i))
					}
				}
				var sum float64
				for _, v := range values {
					sum += v
				}
				want := sum
				switch aggregate {
				case "avg":
					want = sum / float64(len(values))
				case "min":
					want = values[0]
				case "max", "last":
					want = values[len(values)-1]
				}
				if math.Abs(point.Value.(float64)-want) > 0.000001 {
					t.Fatalf("bucket %d: got %v want %v", b, point.Value, want)
				}
			}
		})
	}
}

func TestMonitoringAlertDelayedRecoveryAndObservationGaps(t *testing.T) {
	p := newDashboardTestPlatform(t)
	d := dashboardDevice(t, p, 1)
	now := db.NowMS()
	r := alertRule(t, p, d.ID, now, "threshold")
	alertSample(t, p, d.ID, now, 40)
	alertEvaluate(t, p, now)
	alertExec(t, p, `INSERT INTO telemetry_samples(device_pk,account_id,key,value_num,ts,ingested_at) VALUES(?,1,'temperature',20,?,?)`, d.ID, now-10000, now+1000)
	alertEvaluate(t, p, now+1000)
	alertState(t, p, r.ID, "firing")
	alertExec(t, p, `UPDATE monitoring_rules SET state='normal',for_seconds=30,last_evaluated=0 WHERE id=?`, r.ID)
	alertSample(t, p, d.ID, now+2000, 40)
	alertEvaluate(t, p, now+2000)
	alertState(t, p, r.ID, "pending")
	restarted := New(p.DB, p.Cfg, p.Log)
	alertSample(t, p, d.ID, now+300000, 40)
	alertEvaluate(t, restarted, now+300000)
	if state := alertState(t, p, r.ID, "pending"); state.Since != now+300000 {
		t.Fatal("unobserved gap counted towards sustain")
	}
	alertExec(t, p, `UPDATE devices SET enabled=0 WHERE id=?`, d.ID)
	alertEvaluate(t, p, now+301000)
	alertState(t, p, r.ID, "normal")
	alertExec(t, p, `UPDATE devices SET enabled=1 WHERE id=?`, d.ID)
	alertEvaluate(t, p, now+302000)
	if state := alertState(t, p, r.ID, "pending"); state.Since != now+302000 {
		t.Fatal("disabled interval counted towards sustain")
	}
}

func TestMonitoringAlertDeliveryOrdering(t *testing.T) {
	p := newDashboardTestPlatform(t)
	p.Cfg.Auth.JWTSecret = "integration-ordering-key"
	d := dashboardDevice(t, p, 1)
	now := db.NowMS()
	c := alertChannel(t, p)
	alertRule(t, p, d.ID, now, "threshold", c.ID)
	alertSample(t, p, d.ID, now, 40)
	alertEvaluate(t, p, now)
	first, err := p.monitorClaimDelivery(context.Background(), now)
	if err != nil || first == nil {
		t.Fatalf("first claim %v %v", first, err)
	}
	if err := p.monitorFinishDelivery(first, now, errors.New("transient")); err != nil {
		t.Fatal(err)
	}
	alertSample(t, p, d.ID, now+1000, 20)
	alertEvaluate(t, p, now+1000)
	if next, err := p.monitorClaimDelivery(context.Background(), now+1000); err != nil || next != nil {
		t.Fatalf("resolution bypassed firing retry: %v %v", next, err)
	}
	retry, err := p.monitorClaimDelivery(context.Background(), now+30000)
	if err != nil || retry == nil || retry.id != first.id {
		t.Fatalf("retry order: %v %v", retry, err)
	}
	if err := p.monitorFinishDelivery(retry, now+30000, nil); err != nil {
		t.Fatal(err)
	}
	next, err := p.monitorClaimDelivery(context.Background(), now+30000)
	if err != nil || next == nil || next.id == first.id {
		t.Fatalf("resolution: %v %v", next, err)
	}
}

func TestMonitoringAlertStorageIntegration(t *testing.T) {
	p := newDashboardTestPlatform(t)
	p.Cfg.Auth.JWTSecret = "integration-only-monitoring-secret"
	d := dashboardDevice(t, p, 1)
	now := db.NowMS()
	channel := alertChannel(t, p)
	rule := alertRule(t, p, d.ID, now, "threshold", channel.ID)
	alertSample(t, p, d.ID, now, 40)
	alertEvaluate(t, p, now)
	alertState(t, p, rule.ID, "firing")
	alertDeliveryCount(t, p, 1)
	ctx := context.Background()
	claim, err := p.monitorClaimDelivery(ctx, now)
	if err != nil || claim == nil {
		t.Fatalf("claim: %+v %v", claim, err)
	}
	if duplicate, err := p.monitorClaimDelivery(ctx, now); err != nil || duplicate != nil {
		t.Fatalf("duplicate claim: %+v %v", duplicate, err)
	}
	if err := p.monitorFinishDelivery(claim, now, nil); err != nil {
		t.Fatal(err)
	}
	if ds := alertDeliveryCount(t, p, 1); ds[0].Status != "sent" {
		t.Fatalf("delivery: %+v", ds)
	}
	restarted := New(p.DB, p.Cfg, p.Log)
	alertSample(t, restarted, d.ID, now+61000, 20)
	alertEvaluate(t, restarted, now+61000)
	alertState(t, restarted, rule.ID, "resolved")
	alertDeliveryCount(t, restarted, 2)
	events, err := restarted.ListMonitorEvents(1, 100)
	if err != nil || len(events) != 2 {
		t.Fatalf("events: %+v %v", events, err)
	}
	if events, err := restarted.ListMonitorEvents(2, 100); err != nil || len(events) != 0 {
		t.Fatalf("foreign events: %+v %v", events, err)
	}
	if err := restarted.DeleteMonitorRule(1, rule.ID); err != nil {
		t.Fatal(err)
	}
	if err := restarted.DeleteMonitorChannel(1, channel.ID); err != nil {
		t.Fatal(err)
	}
}

func TestMonitoringConcurrentDashboardQuota(t *testing.T) {
	p := newDashboardTestPlatform(t)
	for i := 0; i < 49; i++ {
		if _, err := p.SaveMonitoringDashboard(1, 0, models.MonitoringDashboard{Name: "Existing"}); err != nil {
			t.Fatal(err)
		}
	}
	start := make(chan struct{})
	results := make(chan error, 12)
	for i := 0; i < 12; i++ {
		go func() {
			<-start
			_, err := p.SaveMonitoringDashboard(1, 0, models.MonitoringDashboard{Name: "Concurrent"})
			results <- err
		}()
	}
	close(start)
	created := 0
	for i := 0; i < 12; i++ {
		err := <-results
		if err == nil {
			created++
		} else if !errors.Is(err, ErrMonitorInvalid) {
			t.Error(err)
		}
	}
	if created != 1 {
		t.Fatalf("concurrent creates=%d, want1", created)
	}
}

func TestMonitoringExactKeysAndNonFiniteSum(t *testing.T) {
	p := newDashboardTestPlatform(t)
	d := dashboardDevice(t, p, 1)
	if err := p.StoreTelemetry(d, []byte(`{"temperature":10," temperature ":20}`)); err != nil {
		t.Fatal(err)
	}
	w := dashboardWidget(d.ID)
	w.Type = "stat"
	w.Sources[0].Key = " temperature "
	got, err := p.QueryMonitoring(1, models.MonitoringQuery{Widget: w})
	if err != nil || got.Series[0].Points[0].Value != float64(20) {
		t.Fatalf("exact key: %+v %v", got, err)
	}
	if p.DB.Dialect == db.DialectPostgres {
		return
	}
	now := db.NowMS()
	for i := 0; i < 2; i++ {
		if _, err := p.DB.Exec(`INSERT INTO telemetry_samples(device_pk,account_id,key,value_num,ts,ingested_at) VALUES(?,1,'huge',?,?,?)`, d.ID, 1e308, now, now); err != nil {
			t.Fatal(err)
		}
	}
	w.Aggregation = "sum"
	w.Sources[0].Key = "huge"
	if _, err := p.QueryMonitoring(1, models.MonitoringQuery{Widget: w}); !errors.Is(err, ErrMonitorInvalid) {
		t.Fatalf("non-finite sum: %v", err)
	}
}

func TestMonitoringConcurrentSnapshot(t *testing.T) {
	p := newDashboardTestPlatform(t)
	d := dashboardDevice(t, p, 1)
	w := dashboardWidget(d.ID)
	w.Title = "Initial"
	dash, err := p.SaveMonitoringDashboard(1, 0, models.MonitoringDashboard{Name: w.Title, Widgets: []models.MonitoringWidget{w}})
	if err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		for i := 0; i < 50; i++ {
			dash.Name = fmt.Sprintf("Version %d", i)
			dash.Widgets[0].Title = dash.Name
			if _, err := p.SaveMonitoringDashboard(1, dash.ID, *dash); err != nil {
				result <- err
				return
			}
			dash.Version++
		}
		result <- nil
	}()
	for i := 0; i < 100; i++ {
		got, err := p.GetMonitoringDashboard(1, dash.ID)
		if err != nil {
			t.Error(err)
			break
		}
		if len(got.Widgets) != 1 || got.Name != got.Widgets[0].Title {
			t.Errorf("inconsistent snapshot: %+v", got)
			break
		}
	}
	if err := <-result; err != nil {
		t.Fatal(err)
	}
}

func TestMonitoringScalarAggregatesWholeWindow(t *testing.T) {
	p := newDashboardTestPlatform(t)
	d := dashboardDevice(t, p, 1)
	from := db.NowMS() - 120000
	for i, value := range []float64{10, 20, 60} {
		if _, err := p.DB.Exec(`INSERT INTO telemetry_samples(device_pk,account_id,key,value_num,ts,ingested_at) VALUES(?,1,'temperature',?,?,?)`, d.ID, value, from+int64(i)*30000, from+int64(i)*30000); err != nil {
			t.Fatal(err)
		}
	}
	for _, kind := range []string{"stat", "gauge"} {
		for aggregate, want := range map[string]float64{"avg": 30, "min": 10, "max": 60, "sum": 90, "last": 60} {
			w := dashboardWidget(d.ID)
			w.Type = kind
			w.Max = 100
			w.Aggregation = aggregate
			got, err := p.QueryMonitoring(1, models.MonitoringQuery{Widget: w, From: from, To: from + 60000})
			if err != nil {
				t.Fatal(err)
			}
			points := got.Series[0].Points
			if len(points) != 1 || points[0].Value != want {
				t.Fatalf("%s %s: %+v want %v", kind, aggregate, points, want)
			}
		}
	}
}

func TestMonitoringDiscoveryLastValuesAndTimeBounds(t *testing.T) {
	p := newDashboardTestPlatform(t)
	d := dashboardDevice(t, p, 1)
	if err := p.StoreTelemetry(d, []byte(`{"temperature":21.5,"ready":true,"status":"ok"}`)); err != nil {
		t.Fatal(err)
	}
	metrics, err := p.MonitoringMetrics(1, d.ID)
	if err != nil || len(metrics) != 3 {
		t.Fatalf("metrics=%+v err=%v", metrics, err)
	}
	wantKinds := map[string]string{"temperature": "number", "ready": "boolean", "status": "text"}
	for _, m := range metrics {
		if m.Kind != wantKinds[m.Key] || m.LastSeen == 0 {
			t.Fatalf("metric: %+v", m)
		}
	}
	if _, err := p.MonitoringMetrics(2, d.ID); !errors.Is(err, ErrMonitorNotFound) {
		t.Fatalf("foreign metrics: %v", err)
	}
	w := dashboardWidget(d.ID)
	w.Aggregation = "last"
	w.Type = "table"
	for key, want := range map[string]any{"temperature": 21.5, "ready": true, "status": "ok"} {
		w.Sources[0].Key = key
		got, err := p.QueryMonitoring(1, models.MonitoringQuery{Widget: w})
		if err != nil || len(got.Series[0].Points) != 1 || got.Series[0].Points[0].Value != want {
			t.Fatalf("%s: %+v, %v", key, got, err)
		}
	}
	w.Sources[0].Key = "absent"
	got, err := p.QueryMonitoring(1, models.MonitoringQuery{Widget: w})
	if err != nil || got.Series[0].Points == nil || len(got.Series[0].Points) != 0 {
		t.Fatalf("no data: %+v, %v", got, err)
	}
	now := db.NowMS()
	for _, bounds := range [][2]int64{{-1, now}, {now, now}, {now, now - 1}, {now, now + 120000}, {now - int64(monitorMaxWindow)*1000 - 1, now}} {
		if _, err := p.QueryMonitoring(1, models.MonitoringQuery{Widget: w, From: bounds[0], To: bounds[1]}); !errors.Is(err, ErrMonitorInvalid) {
			t.Fatalf("bounds %v: %v", bounds, err)
		}
	}
	for _, v := range []string{"first", "second"} {
		if _, err := p.DB.Exec(`INSERT INTO telemetry_samples(device_pk,account_id,key,value_text,ts,ingested_at) VALUES(?,1,'tie',?,?,?)`, d.ID, v, now, now); err != nil {
			t.Fatal(err)
		}
	}
	w.Sources[0].Key = "tie"
	got, err = p.QueryMonitoring(1, models.MonitoringQuery{Widget: w, From: now - 60000, To: now})
	if err != nil || len(got.Series[0].Points) != 1 || got.Series[0].Points[0].Value != "second" {
		t.Fatalf("timestamp tie: %+v, %v", got, err)
	}
}
