package services

import (
	"log/slog"
	"path/filepath"
	"testing"

	"cogitor-iot-platform/internal/config"
	"cogitor-iot-platform/internal/db"
)

// newTestPlatform creates a Platform with a fresh SQLite database,
// runs migrations, and inserts a default account (id=1) so that
// foreign key constraints on products.account_id are satisfied.
func newTestPlatform(t *testing.T, cfg config.Config) *Platform {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	database, err := db.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	// db.Open already runs migrations, so tables are created

	// Insert default account so FK constraints are satisfied
	_, err = database.Exec(`INSERT INTO accounts(id, slug, name, status, created_at, updated_at) VALUES(1, 'default', 'Default Account', 'active', ?, ?)`,
		db.NowMS(), db.NowMS())
	if err != nil {
		t.Fatalf("insert default account: %v", err)
	}

	if cfg.DataDir == "" {
		cfg.DataDir = dir
	}
	if cfg.Database.Driver == "" {
		cfg.Database.Driver = "sqlite"
		cfg.Database.DSN = dbPath
	}

	log := slog.New(slog.DiscardHandler)
	return New(database, cfg, log)
}
