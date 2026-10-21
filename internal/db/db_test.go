package db

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
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
	// SQLite must not transform SQL (directive parsing only re-joins lines)
	input := "INSERT OR IGNORE INTO t VALUES (1); PRAGMA foreign_keys = ON;"
	result := adaptSQL(input, DialectSQLite)
	if strings.TrimRight(result, "\n") != input {
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

func TestApplyDirectives(t *testing.T) {
	input := `-- shared comment
CREATE TABLE t (id INTEGER);
-- sqlite:begin
PRAGMA foreign_keys=OFF;
CREATE TABLE t2 (a INTEGER);
-- sqlite:end
-- pg:begin
ALTER TABLE t ADD COLUMN b INTEGER;
-- pg:end
CREATE INDEX idx ON t(id);
`
	sqliteOut := applyDirectives(input, DialectSQLite)
	if strings.Contains(sqliteOut, "ALTER TABLE t ADD COLUMN") {
		t.Errorf("sqlite output must not contain pg block: %s", sqliteOut)
	}
	if !strings.Contains(sqliteOut, "PRAGMA foreign_keys=OFF") || !strings.Contains(sqliteOut, "CREATE TABLE t2") {
		t.Errorf("sqlite output must keep sqlite block: %s", sqliteOut)
	}
	if !strings.Contains(sqliteOut, "CREATE TABLE t (id INTEGER)") || !strings.Contains(sqliteOut, "CREATE INDEX idx") {
		t.Errorf("sqlite output must keep shared content: %s", sqliteOut)
	}

	pgOut := applyDirectives(input, DialectPostgres)
	if strings.Contains(pgOut, "PRAGMA") || strings.Contains(pgOut, "CREATE TABLE t2") {
		t.Errorf("pg output must not contain sqlite block: %s", pgOut)
	}
	if !strings.Contains(pgOut, "ALTER TABLE t ADD COLUMN") {
		t.Errorf("pg output must keep pg block: %s", pgOut)
	}
}

// TestMigrateAlinkRebuild verifies 002_alink on SQLite: pre-existing rows
// survive the devices/products table rebuild, the CHECK accepts 'alink',
// alink_secret exists, indexes are recreated, and foreign keys are
// re-enabled afterwards.
func TestMigrateAlinkRebuild(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rebuild.db")
	dsn := fmt.Sprintf("file:%s?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)", filepath.ToSlash(path))
	raw, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	raw.SetMaxOpenConns(1)

	// Apply 001 only, then insert data the rebuild must preserve.
	body001, err := migrationFS.ReadFile("migrations/001_init.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(adaptSQL(string(body001), DialectSQLite)); err != nil {
		t.Fatalf("apply 001: %v", err)
	}
	now := NowMS()
	if _, err := raw.Exec(`INSERT INTO accounts(id, slug, name, status, created_at, updated_at) VALUES(1,'acc','acc','active',?,?)`, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`INSERT INTO products(id, account_id, product_id, name, topic_style_default, created_at, updated_at) VALUES(1,1,'pk1','p1','full',?,?)`, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`INSERT INTO devices(id, product_pk, device_id, name, topic_style, fw_version, status, created_at, updated_at) VALUES(1,1,'dev1','d1','short','1.0.0','online',?,?)`, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`INSERT INTO telemetry_samples(device_pk, account_id, key, value_num, ts, ingested_at) VALUES(1,1,'temp',21.5,?,?)`, now, now); err != nil {
		t.Fatal(err)
	}

	// Run pending migrations (002) with data present.
	if err := migrate(raw, DialectSQLite); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	var fw, style, status string
	var lastSeen sql.NullInt64
	if err := raw.QueryRow(`SELECT fw_version, topic_style, status, last_seen_at FROM devices WHERE id=1`).Scan(&fw, &style, &status, &lastSeen); err != nil {
		t.Fatalf("device row lost after rebuild: %v", err)
	}
	if fw != "1.0.0" || style != "short" || status != "online" || lastSeen.Valid {
		t.Errorf("device row mutated: fw=%q style=%q status=%q last_seen=%+v", fw, style, status, lastSeen)
	}
	var n int
	if err := raw.QueryRow(`SELECT COUNT(1) FROM telemetry_samples WHERE device_pk=1`).Scan(&n); err != nil || n != 1 {
		t.Errorf("telemetry rows after rebuild: n=%d err=%v", n, err)
	}

	// New columns and CHECK values.
	if _, err := raw.Exec(`UPDATE devices SET topic_style='alink', alink_secret='sec' WHERE id=1`); err != nil {
		t.Errorf("alink style/secret rejected: %v", err)
	}
	if _, err := raw.Exec(`UPDATE products SET topic_style_default='alink' WHERE id=1`); err != nil {
		t.Errorf("products alink default rejected: %v", err)
	}
	if _, err := raw.Exec(`UPDATE devices SET topic_style='bogus' WHERE id=1`); err == nil {
		t.Error("bogus topic_style accepted")
	}

	// Indexes recreated by the rebuild.
	for _, idx := range []string{"idx_devices_status", "idx_devices_token_prefix", "idx_products_account"} {
		if err := raw.QueryRow(`SELECT COUNT(1) FROM sqlite_master WHERE type='index' AND name=?`, idx).Scan(&n); err != nil || n != 1 {
			t.Errorf("index %s missing after rebuild (n=%d err=%v)", idx, n, err)
		}
	}

	// Foreign keys re-enabled: child insert with bogus parent must fail.
	if _, err := raw.Exec(`INSERT INTO telemetry_samples(device_pk, account_id, key, value_num, ts, ingested_at) VALUES(999,1,'x',1,?,?)`, now, now); err == nil {
		t.Error("FK not enforced after migration (foreign_keys not restored)")
	}
}

// TestMigrateFreshAlink verifies a fresh database (001+002 together) accepts
// the alink dialect end-to-end.
func TestMigrateFreshAlink(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fresh.db")
	database, err := Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	now := NowMS()
	if _, err := database.Exec(`INSERT INTO accounts(id, slug, name, status, created_at, updated_at) VALUES(1,'acc','acc','active',?,?)`, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO products(account_id, product_id, name, topic_style_default, created_at, updated_at) VALUES(1,'pk1','p1','alink',?,?)`, now, now); err != nil {
		t.Fatalf("fresh install must accept topic_style_default='alink': %v", err)
	}
	res, err := database.Exec(`INSERT INTO devices(product_pk, device_id, topic_style, alink_secret, created_at, updated_at) VALUES(1,'dev1','alink','s3cr3t',?,?)`, now, now)
	if err != nil {
		t.Fatalf("fresh install must accept topic_style='alink': %v", err)
	}
	if _, err := res.LastInsertId(); err != nil {
		t.Errorf("LastInsertId after rebuild: %v", err)
	}
	var secret string
	if err := database.QueryRow(`SELECT alink_secret FROM devices WHERE product_pk=1 AND device_id='dev1'`).Scan(&secret); err != nil || secret != "s3cr3t" {
		t.Errorf("alink_secret roundtrip: %q err=%v", secret, err)
	}
}

// TestMigrateThingsBoardRebuild verifies 003_thingsboard on SQLite: rows
// survive the devices/products rebuild (after 002 already ran), the CHECK
// accepts 'thingsboard', and foreign keys are re-enabled afterwards.
func TestMigrateThingsBoardRebuild(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rebuild.db")
	dsn := fmt.Sprintf("file:%s?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)", filepath.ToSlash(path))
	raw, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	raw.SetMaxOpenConns(1)

	// Apply 001 only, then insert data both rebuilds must preserve.
	body001, err := migrationFS.ReadFile("migrations/001_init.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(adaptSQL(string(body001), DialectSQLite)); err != nil {
		t.Fatalf("apply 001: %v", err)
	}
	now := NowMS()
	if _, err := raw.Exec(`INSERT INTO accounts(id, slug, name, status, created_at, updated_at) VALUES(1,'acc','acc','active',?,?)`, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`INSERT INTO products(id, account_id, product_id, name, topic_style_default, created_at, updated_at) VALUES(1,1,'pk1','p1','full',?,?)`, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`INSERT INTO devices(id, product_pk, device_id, name, topic_style, fw_version, status, created_at, updated_at) VALUES(1,1,'dev1','d1','short','1.0.0','online',?,?)`, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`INSERT INTO telemetry_samples(device_pk, account_id, key, value_num, ts, ingested_at) VALUES(1,1,'temp',21.5,?,?)`, now, now); err != nil {
		t.Fatal(err)
	}

	// Run pending migrations (002+003) with data present.
	if err := migrate(raw, DialectSQLite); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	var fw, style, status string
	if err := raw.QueryRow(`SELECT fw_version, topic_style, status FROM devices WHERE id=1`).Scan(&fw, &style, &status); err != nil {
		t.Fatalf("device row lost after rebuild: %v", err)
	}
	if fw != "1.0.0" || style != "short" || status != "online" {
		t.Errorf("device row mutated: fw=%q style=%q status=%q", fw, style, status)
	}
	var n int
	if err := raw.QueryRow(`SELECT COUNT(1) FROM telemetry_samples WHERE device_pk=1`).Scan(&n); err != nil || n != 1 {
		t.Errorf("telemetry rows after rebuild: n=%d err=%v", n, err)
	}

	// The CHECK now accepts 'thingsboard' on both tables.
	if _, err := raw.Exec(`UPDATE devices SET topic_style='thingsboard' WHERE id=1`); err != nil {
		t.Errorf("thingsboard style rejected: %v", err)
	}
	if _, err := raw.Exec(`UPDATE products SET topic_style_default='thingsboard' WHERE id=1`); err != nil {
		t.Errorf("products thingsboard default rejected: %v", err)
	}
	if _, err := raw.Exec(`UPDATE devices SET topic_style='bogus' WHERE id=1`); err == nil {
		t.Error("bogus topic_style accepted")
	}

	// Foreign keys re-enabled: child insert with bogus parent must fail.
	if _, err := raw.Exec(`INSERT INTO telemetry_samples(device_pk, account_id, key, value_num, ts, ingested_at) VALUES(999,1,'x',1,?,?)`, now, now); err == nil {
		t.Error("FK not enforced after migration (foreign_keys not restored)")
	}
}
