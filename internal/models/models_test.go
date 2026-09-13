package models

import "testing"

func TestRoleConstants(t *testing.T) {
	if RoleAdmin != "admin" {
		t.Errorf("RoleAdmin = %q, want 'admin'", RoleAdmin)
	}
	if RoleOperator != "operator" {
		t.Errorf("RoleOperator = %q, want 'operator'", RoleOperator)
	}
	if RoleViewer != "viewer" {
		t.Errorf("RoleViewer = %q, want 'viewer'", RoleViewer)
	}
}

func TestDeviceStatus(t *testing.T) {
	// Test that device status constants are used correctly
	d := Device{
		ID:       1,
		DeviceID: "test-device",
		Status:   "online",
	}
	if d.Status != "online" {
		t.Errorf("Device.Status = %q, want 'online'", d.Status)
	}
}

func TestOTAJobStatus(t *testing.T) {
	// Test OTA job struct
	job := OTAJob{
		ID:          1,
		DevicePK:    100,
		ProductID:   "test-product",
		DeviceID:    "test-device",
		Status:      "pending",
		Description: "Test OTA update",
	}
	if job.Status != "pending" {
		t.Errorf("OTAJob.Status = %q, want 'pending'", job.Status)
	}
	if job.Description != "Test OTA update" {
		t.Errorf("OTAJob.Description = %q, want 'Test OTA update'", job.Description)
	}
}

func TestTelemetrySample(t *testing.T) {
	// Test numeric value
	numVal := 42.5
	sample := TelemetrySample{
		Key:      "temperature",
		ValueNum: &numVal,
		TS:       1234567890,
	}
	if sample.Key != "temperature" {
		t.Errorf("Key = %q, want 'temperature'", sample.Key)
	}
	if sample.ValueNum == nil || *sample.ValueNum != 42.5 {
		t.Errorf("ValueNum = %v, want 42.5", sample.ValueNum)
	}

	// Test text value
	textVal := "active"
	sample2 := TelemetrySample{
		Key:       "status",
		ValueText: &textVal,
		TS:        1234567890,
	}
	if sample2.ValueText == nil || *sample2.ValueText != "active" {
		t.Errorf("ValueText = %v, want 'active'", sample2.ValueText)
	}

	// Test bool value
	boolVal := true
	sample3 := TelemetrySample{
		Key:       "online",
		ValueBool: &boolVal,
		TS:        1234567890,
	}
	if sample3.ValueBool == nil || !*sample3.ValueBool {
		t.Errorf("ValueBool = %v, want true", sample3.ValueBool)
	}
}

func TestExternalPlugin(t *testing.T) {
	ep := ExternalPlugin{
		ID:          "test-plugin",
		AccountID:   1,
		Name:        "Test Plugin",
		Description: "A test plugin",
		Status:      "unknown",
		MQTTUser:    "extplg_acme_test-plugin",
	}
	if ep.ID != "test-plugin" {
		t.Errorf("ID = %q, want 'test-plugin'", ep.ID)
	}
	if ep.Status != "unknown" {
		t.Errorf("Status = %q, want 'unknown'", ep.Status)
	}
	// MQTTPassHash should not be serialized (json:"-")
	if ep.MQTTPassHash != "" {
		t.Error("MQTTPassHash should be empty by default")
	}
}
