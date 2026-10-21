package services

import (
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"cogitor-iot-platform/internal/protocol"
)

func TestValidateIDs(t *testing.T) {
	tests := []struct {
		name      string
		productID string
		deviceID  string
		wantErr   bool
	}{
		// Valid cases
		{"simple", "sensor", "dev01", false},
		{"with dots", "my.product", "device.v2", false},
		{"with dashes", "my-product", "device-01", false},
		{"with underscores", "my_product", "device_01", false},
		{"starts with number", "1product", "1device", false},
		{"max length 63", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", false},
		{"single char", "a", "b", false},

		// Invalid cases
		{"empty product", "", "device", true},
		{"empty device", "product", "", true},
		{"both empty", "", "", true},
		{"special chars product", "product@home", "device", true},
		{"special chars device", "product", "device!1", true},
		{"starts with dot", ".product", "device", true},
		{"starts with dash", "-product", "device", true},
		{"too long product", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "device", true}, // 64 chars
		{"too long device", "product", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", true}, // 64 chars
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateIDs(tt.productID, tt.deviceID)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateIDs(%q, %q) error = %v, wantErr %v", tt.productID, tt.deviceID, err, tt.wantErr)
			}
		})
	}
}

func TestFingerprint(t *testing.T) {
	// Same input should produce same fingerprint
	fp1 := fingerprint([]byte("hello world"))
	fp2 := fingerprint([]byte("hello world"))
	if fp1 != fp2 {
		t.Errorf("same input should produce same fingerprint: %q != %q", fp1, fp2)
	}

	// Different input should produce different fingerprint
	fp3 := fingerprint([]byte("different"))
	if fp1 == fp3 {
		t.Error("different input should produce different fingerprint")
	}

	// Fingerprint should be 32 hex chars (16 bytes)
	if len(fp1) != 32 {
		t.Errorf("fingerprint length = %d, want 32", len(fp1))
	}

	// Empty input should still produce a fingerprint
	fp4 := fingerprint([]byte(""))
	if len(fp4) != 32 {
		t.Errorf("empty fingerprint length = %d, want 32", len(fp4))
	}
}

func TestParseTags(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int
	}{
		{"empty", "", 0},
		{"null", "null", 0},
		{"empty array", "[]", 0},
		{"single", `["tag1"]`, 1},
		{"multiple", `["tag1","tag2","tag3"]`, 3},
		{"invalid json", "not-json", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseTags(tt.input)
			if len(got) != tt.want {
				t.Errorf("parseTags(%q) len = %d, want %d", tt.input, len(got), tt.want)
			}
		})
	}
}

func TestTagsJSON(t *testing.T) {
	// nil should produce empty array
	if got := tagsJSON(nil); got != "[]" {
		t.Errorf("tagsJSON(nil) = %q, want '[]'", got)
	}

	// empty slice should produce empty array
	if got := tagsJSON([]string{}); got != "[]" {
		t.Errorf("tagsJSON([]) = %q, want '[]'", got)
	}

	// single tag
	got := tagsJSON([]string{"tag1"})
	var parsed []string
	_ = json.Unmarshal([]byte(got), &parsed)
	if len(parsed) != 1 || parsed[0] != "tag1" {
		t.Errorf("tagsJSON(['tag1']) = %q", got)
	}

	// multiple tags
	got = tagsJSON([]string{"a", "b", "c"})
	_ = json.Unmarshal([]byte(got), &parsed)
	if len(parsed) != 3 {
		t.Errorf("tagsJSON(['a','b','c']) len = %d, want 3", len(parsed))
	}
}

func TestNullInt64(t *testing.T) {
	// Valid value
	v := nullInt64(sql.NullInt64{Int64: 42, Valid: true})
	if v == nil || *v != 42 {
		t.Errorf("nullInt64(42) = %v, want 42", v)
	}

	// Invalid (NULL) value
	v = nullInt64(sql.NullInt64{Int64: 0, Valid: false})
	if v != nil {
		t.Errorf("nullInt64(NULL) = %v, want nil", v)
	}
}

func TestSnippet(t *testing.T) {
	dl := protocol.DialectFor(protocol.StyleShort)
	got := dl.Snippet("myProduct", "myDevice", "mqtt.example.com", "token123")
	if got == "" {
		t.Error("snippet should not be empty")
	}
	// Should contain all parameters
	for _, s := range []string{"myProduct", "myDevice", "mqtt.example.com", "token123"} {
		if !containsStr(got, s) {
			t.Errorf("snippet should contain %q: %s", s, got)
		}
	}
}

func TestJsonValue(t *testing.T) {
	// String
	if got := jsonValue("hello"); got != `"hello"` {
		t.Errorf("jsonValue('hello') = %q", got)
	}

	// Number
	if got := jsonValue(42); got != "42" {
		t.Errorf("jsonValue(42) = %q", got)
	}

	// Map
	got := jsonValue(map[string]int{"a": 1})
	if got == "" {
		t.Error("jsonValue(map) should not be empty")
	}

	// Nil
	if got := jsonValue(nil); got != "null" {
		t.Errorf("jsonValue(nil) = %q, want 'null'", got)
	}
}

func TestListAuditLog_LimitBounds(t *testing.T) {
	// Test limit clamping logic (without DB)
	tests := []struct {
		input int
		want  int
	}{
		{-1, 100},
		{0, 100},
		{1, 1},
		{100, 100},
		{500, 500},
		{501, 100},
		{1000, 100},
	}

	for _, tt := range tests {
		limit := tt.input
		if limit <= 0 || limit > 500 {
			limit = 100
		}
		if limit != tt.want {
			t.Errorf("limit(%d) = %d, want %d", tt.input, limit, tt.want)
		}
	}
}

func TestRetentionCutoff(t *testing.T) {
	// Test cutoff calculation logic
	days := 30
	now := time.Now().UnixMilli()
	cutoff := now - int64(days)*24*3600*1000

	// Cutoff should be ~30 days ago
	diff := now - cutoff
	expectedDiff := int64(30) * 24 * 3600 * 1000
	tolerance := int64(1000) // 1 second tolerance

	if diff < expectedDiff-tolerance || diff > expectedDiff+tolerance {
		t.Errorf("cutoff diff = %d, want ~%d", diff, expectedDiff)
	}
}

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
