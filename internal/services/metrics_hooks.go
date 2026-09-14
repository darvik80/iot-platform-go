package services

import (
	"fmt"

	"cogitor-iot-platform/internal/plugins"
)

// Metrics helper functions that build label strings and call the global
// MetricsStore. These are called from service methods to keep the prometheus
// exporter counters in sync with platform activity.

func metricLabel(pairs ...string) string {
	// Build "key1=val1,key2=val2" from alternating key/value pairs
	var s string
	for i := 0; i+1 < len(pairs); i += 2 {
		if s != "" {
			s += ","
		}
		s += pairs[i] + "=" + pairs[i+1]
	}
	return s
}

// MetricTelemetrySample increments the telemetry samples counter.
func MetricTelemetrySample(account, product, device string) {
	labels := metricLabel("account", account, "product", product, "device", device)
	plugins.Metrics.IncrCounter(fmt.Sprintf("cogitor_device_telemetry_samples_total{%s}", labels))
}

// MetricRPCCall increments the RPC calls counter.
func MetricRPCCall(account, product, device, method, status string) {
	labels := metricLabel("account", account, "product", product, "device", device, "method", method, "status", status)
	plugins.Metrics.IncrCounter(fmt.Sprintf("cogitor_device_rpc_calls_total{%s}", labels))
}

// MetricEventBusPublished increments the event bus published counter.
func MetricEventBusPublished(account, product, device, kind string) {
	labels := metricLabel("account", account, "product", product, "device", device, "kind", kind)
	plugins.Metrics.IncrCounter(fmt.Sprintf("cogitor_event_bus_published_total{%s}", labels))
}

// MetricDeviceOnline sets the device online gauge (1=online, 0=offline).
func MetricDeviceOnline(account, product, device string, online bool) {
	labels := metricLabel("account", account, "product", product, "device", device)
	v := int64(0)
	if online {
		v = 1
	}
	plugins.Metrics.SetGauge(fmt.Sprintf("cogitor_device_online{%s}", labels), v)
}

// MetricDevicesTotal sets the total devices gauge for a given account/product/status.
func MetricDevicesTotal(account, product, status string, count int64) {
	labels := metricLabel("account", account, "product", product, "status", status)
	plugins.Metrics.SetGaugeValue(fmt.Sprintf("cogitor_devices_total{%s}", labels), count)
}

// MetricMQTTClients sets the MQTT clients gauge for a given account/kind.
func MetricMQTTClients(account, kind string, count int64) {
	labels := metricLabel("account", account, "kind", kind)
	plugins.Metrics.SetGaugeValue(fmt.Sprintf("cogitor_mqtt_clients{%s}", labels), count)
}

// MetricExternalPluginOnline sets the external plugin online gauge.
func MetricExternalPluginOnline(account, pluginID string, online bool) {
	labels := metricLabel("account", account, "plugin_id", pluginID)
	v := int64(0)
	if online {
		v = 1
	}
	plugins.Metrics.SetGauge(fmt.Sprintf("cogitor_external_plugin_online{%s}", labels), v)
}

// MetricOTAJob increments the OTA jobs counter.
func MetricOTAJob(account, product, device, status string) {
	labels := metricLabel("account", account, "product", product, "device", device, "status", status)
	plugins.Metrics.IncrCounter(fmt.Sprintf("cogitor_device_ota_jobs_total{%s}", labels))
}
