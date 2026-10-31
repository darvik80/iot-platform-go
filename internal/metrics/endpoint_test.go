package metrics

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"cogitor-iot-platform/internal/db"
)

func TestSmokeMetricsEndpoint(t *testing.T) {
	database, err := db.Open("sqlite", filepath.Join(t.TempDir(), "smoke.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	mustExec := func(q string, args ...any) {
		t.Helper()
		if _, err := database.Exec(q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	mustExec(`INSERT INTO accounts(id,slug,name,status,created_at,updated_at) VALUES(1,'acme','Acme','active',1,1)`)
	mustExec(`INSERT INTO products(id,account_id,product_id,name,created_at,updated_at) VALUES(1,1,'sensor','Sensor',1,1)`)
	mustExec(`INSERT INTO devices(id,product_pk,device_id,name,status,created_at,updated_at) VALUES(1,1,'d1','D1','online',1,1)`)
	mustExec(`INSERT INTO devices(id,product_pk,device_id,name,status,created_at,updated_at) VALUES(2,1,'d2','D2','offline',1,1)`)

	RegisterDB(database.Raw)
	RegisterDomain(database, nil)
	SetMQTTClientProvider(func() map[string]int { return map[string]int{"device": 1, "plugin": 2} })
	SetBuildInfo("smoke", "deadbeef")

	r := chi.NewRouter()
	r.Use(Middleware)
	r.Get("/api/v1/devices/{id}", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/devices/1", nil))

	rec := httptest.NewRecorder()
	Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body := rec.Body.String()

	for _, want := range []string{
		"platform_http_requests_total{method=\"GET\",route=\"/api/v1/devices/{id}\",status=\"200\"}",
		"platform_build_info{commit=\"deadbeef\",version=\"smoke\"} 1",
		"platform_devices{account=\"acme\",status=\"offline\"} 1",
		"platform_devices{account=\"acme\",status=\"online\"} 1",
		"platform_mqtt_clients{kind=\"device\"} 1",
		"platform_mqtt_clients{kind=\"plugin\"} 2",
		"go_sql_open_connections{db_name=\"platform\"}",
		"go_goroutines",
		"process_start_time_seconds",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
	t.Logf("samples=%d", strings.Count(body, "\n"))
}
