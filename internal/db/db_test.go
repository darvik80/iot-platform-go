package db

import (
	"strings"
	"testing"
	"time"
)

func TestConvertPlaceholders_SQLite(t *testing.T) {
	// SQLite should not convert ? to $N
	query := "SELECT * FROM users WHERE id = ? AND name = ?"
	result := convertPlaceholders(query)
	// For SQLite, we don't call convertPlaceholders, but let's test the function directly
	// It should always convert (the caller decides when to call it)
	if result != "SELECT * FROM users WHERE id = $1 AND name = $2" {
		t.Errorf("expected $1, $2, got: %s", result)
	}
}

func TestConvertPlaceholders_Postgres(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "single placeholder",
			input:    "SELECT * FROM users WHERE id = ?",
			expected: "SELECT * FROM users WHERE id = $1",
		},
		{
			name:     "multiple placeholders",
			input:    "INSERT INTO users (name, email) VALUES (?, ?)",
			expected: "INSERT INTO users (name, email) VALUES ($1, $2)",
		},
		{
			name:     "no placeholders",
			input:    "SELECT 1",
			expected: "SELECT 1",
		},
		{
			name:     "placeholder in string literal should still convert",
			input:    "SELECT * FROM t WHERE name = '?' AND id = ?",
			expected: "SELECT * FROM t WHERE name = '$1' AND id = $2",
		},
		{
			name:     "complex query",
			input:    "UPDATE t SET a = ?, b = ? WHERE c = ? AND d IN (?, ?)",
			expected: "UPDATE t SET a = $1, b = $2 WHERE c = $3 AND d IN ($4, $5)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertPlaceholders(tt.input)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestConvertInsertOrIgnore(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple insert or ignore",
			input:    "INSERT OR IGNORE INTO settings (key, value) VALUES ('a', 'b');",
			expected: "INSERT INTO settings (key, value) VALUES ('a', 'b') ON CONFLICT DO NOTHING;",
		},
		{
			name:     "multiple statements",
			input:    "INSERT OR IGNORE INTO t1 VALUES (1); INSERT OR IGNORE INTO t2 VALUES (2);",
			expected: "INSERT INTO t1 VALUES (1) ON CONFLICT DO NOTHING; INSERT INTO t2 VALUES (2) ON CONFLICT DO NOTHING;",
		},
		{
			name:     "no insert or ignore",
			input:    "INSERT INTO t VALUES (1);",
			expected: "INSERT INTO t VALUES (1);",
		},
		{
			name:     "case insensitive",
			input:    "insert or ignore into t values (1);",
			expected: "INSERT INTO t values (1) ON CONFLICT DO NOTHING;",
		},
		{
			name:     "no semicolon",
			input:    "INSERT OR IGNORE INTO t VALUES (1)",
			expected: "INSERT INTO t VALUES (1) ON CONFLICT DO NOTHING",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertInsertOrIgnore(tt.input)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestIsInsert(t *testing.T) {
	tests := []struct {
		query    string
		expected bool
	}{
		{"INSERT INTO users VALUES (1)", true},
		{"insert into users values (1)", true},
		{"  INSERT INTO users VALUES (1)", true},
		{"SELECT * FROM users", false},
		{"UPDATE users SET name = 'x'", false},
		{"DELETE FROM users", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			if got := isInsert(tt.query); got != tt.expected {
				t.Errorf("isInsert(%q) = %v, want %v", tt.query, got, tt.expected)
			}
		})
	}
}

func TestHasReturning(t *testing.T) {
	tests := []struct {
		query    string
		expected bool
	}{
		{"INSERT INTO t VALUES (1) RETURNING id", true},
		{"INSERT INTO t VALUES (1) returning id", true},
		{"INSERT INTO t VALUES (1)", false},
		{"SELECT * FROM t RETURNING id", true}, // has RETURNING keyword
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			if got := hasReturning(tt.query); got != tt.expected {
				t.Errorf("hasReturning(%q) = %v, want %v", tt.query, got, tt.expected)
			}
		})
	}
}

func TestAdaptSQL_SQLite(t *testing.T) {
	// SQLite should not modify the SQL
	input := "INSERT OR IGNORE INTO t VALUES (1); PRAGMA foreign_keys = ON;"
	result := adaptSQL(input, DialectSQLite)
	if result != input {
		t.Errorf("SQLite should not modify SQL, got: %s", result)
	}
}

func TestAdaptSQL_Postgres(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		contains []string
		excludes []string
	}{
		{
			name:     "remove PRAGMA",
			input:    "PRAGMA foreign_keys = ON; CREATE TABLE t (id INTEGER);",
			contains: []string{"CREATE TABLE"},
			excludes: []string{"PRAGMA"},
		},
		{
			name:     "convert AUTOINCREMENT",
			input:    "CREATE TABLE t (id INTEGER PRIMARY KEY AUTOINCREMENT);",
			contains: []string{"BIGSERIAL PRIMARY KEY"},
			excludes: []string{"AUTOINCREMENT"},
		},
		{
			name:     "convert INSERT OR IGNORE",
			input:    "INSERT OR IGNORE INTO t VALUES (1);",
			contains: []string{"ON CONFLICT DO NOTHING"},
			excludes: []string{"INSERT OR IGNORE"},
		},
		{
			name:     "convert placeholders",
			input:    "SELECT * FROM t WHERE a = ? AND b = ?",
			contains: []string{"$1", "$2"},
			excludes: []string{"?"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := adaptSQL(tt.input, DialectPostgres)
			for _, s := range tt.contains {
				if !strings.Contains(result, s) {
					t.Errorf("expected to contain %q, got: %s", s, result)
				}
			}
			for _, s := range tt.excludes {
				if strings.Contains(result, s) {
					t.Errorf("expected NOT to contain %q, got: %s", s, result)
				}
			}
		})
	}
}

func TestResult_LastInsertId(t *testing.T) {
	// Test with hasID = true (PostgreSQL RETURNING path)
	r := Result{pg: true, lastID: 42, hasID: true}
	id, err := r.LastInsertId()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if id != 42 {
		t.Errorf("expected 42, got %d", id)
	}

	// Test with nil inner and no ID
	r2 := Result{pg: false, inner: nil}
	_, err = r2.LastInsertId()
	if err == nil {
		t.Error("expected error for nil result")
	}
}

func TestNowMS(t *testing.T) {
	// NowMS() returns milliseconds, so we just verify it returns a reasonable value
	now := NowMS()
	if now <= 0 {
		t.Errorf("NowMS() should return positive value, got %d", now)
	}
	// Verify it's roughly current time (within last minute)
	expectedMin := time.Now().UnixMilli() - 60000
	if now < expectedMin {
		t.Errorf("NowMS() = %d, expected >= %d", now, expectedMin)
	}
}
