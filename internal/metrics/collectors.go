package metrics

import (
	"database/sql"
	"log/slog"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

// Querier is the subset of db.DB the domain collector needs.
type Querier interface {
	Query(query string, args ...any) (*sql.Rows, error)
}

var (
	devicesDesc = prometheus.NewDesc(
		"platform_devices",
		"Devices by account and status.",
		[]string{"account", "status"}, nil)

	otaJobsDesc = prometheus.NewDesc(
		"platform_ota_jobs",
		"OTA jobs by account and status.",
		[]string{"account", "status"}, nil)

	externalPluginsDesc = prometheus.NewDesc(
		"platform_external_plugins",
		"External plugins by account and status.",
		[]string{"account", "status"}, nil)

	mqttClientsDesc = prometheus.NewDesc(
		"platform_mqtt_clients",
		"Connected MQTT clients by kind (device, plugin, other).",
		[]string{"kind"}, nil)

	mqttMu       sync.RWMutex
	mqttProvider func() map[string]int
)

func init() {
	Registry.MustRegister(mqttCollector{})
}

// RegisterDB exposes database/sql pool statistics as go_sql_*{db_name="platform"}.
func RegisterDB(raw *sql.DB) {
	if raw == nil {
		return
	}
	Registry.MustRegister(collectors.NewDBStatsCollector(raw, "platform"))
}

// RegisterDomain exposes state that is cheap to read on scrape: devices, OTA
// jobs and external plugins grouped by status. Reading from the database
// instead of tracking events keeps the gauges correct across restarts.
func RegisterDomain(q Querier, log *slog.Logger) {
	if q == nil {
		return
	}
	Registry.MustRegister(&domainCollector{q: q, log: log})
}

// SetMQTTClientProvider installs the broker's client-count snapshot. It is
// called once the broker exists; scraping before that reports no series.
func SetMQTTClientProvider(f func() map[string]int) {
	mqttMu.Lock()
	mqttProvider = f
	mqttMu.Unlock()
}

type mqttCollector struct{}

func (mqttCollector) Describe(ch chan<- *prometheus.Desc) { ch <- mqttClientsDesc }

func (mqttCollector) Collect(ch chan<- prometheus.Metric) {
	mqttMu.RLock()
	f := mqttProvider
	mqttMu.RUnlock()
	if f == nil {
		return
	}
	for kind, n := range f() {
		ch <- prometheus.MustNewConstMetric(mqttClientsDesc, prometheus.GaugeValue, float64(n), kind)
	}
}

type domainCollector struct {
	q   Querier
	log *slog.Logger
}

func (c *domainCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- devicesDesc
	ch <- otaJobsDesc
	ch <- externalPluginsDesc
}

func (c *domainCollector) Collect(ch chan<- prometheus.Metric) {
	c.emit(ch, devicesDesc, `SELECT COALESCE(a.slug, ''), d.status, COUNT(*)
		FROM devices d
		JOIN products p ON p.id = d.product_pk
		LEFT JOIN accounts a ON a.id = p.account_id
		GROUP BY 1, 2`, "devices")
	c.emit(ch, otaJobsDesc, `SELECT COALESCE(a.slug, ''), j.status, COUNT(*)
		FROM ota_jobs j
		LEFT JOIN accounts a ON a.id = j.account_id
		GROUP BY 1, 2`, "ota_jobs")
	c.emit(ch, externalPluginsDesc, `SELECT COALESCE(a.slug, ''), e.status, COUNT(*)
		FROM external_plugins e
		LEFT JOIN accounts a ON a.id = e.account_id
		GROUP BY 1, 2`, "external_plugins")
}

func (c *domainCollector) emit(ch chan<- prometheus.Metric, desc *prometheus.Desc, query, name string) {
	rows, err := c.q.Query(query)
	if err != nil {
		c.debug("metrics query failed", name, err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var account, status string
		var n int64
		if err := rows.Scan(&account, &status, &n); err != nil {
			c.debug("metrics scan failed", name, err)
			continue
		}
		ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, float64(n), account, status)
	}
	if err := rows.Err(); err != nil {
		c.debug("metrics rows failed", name, err)
	}
}

func (c *domainCollector) debug(msg, name string, err error) {
	if c.log != nil {
		c.log.Debug(msg, "metric", name, "err", err)
	}
}
