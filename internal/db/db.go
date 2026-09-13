package db

import (
	"database/sql"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// Dialect represents the database dialect.
type Dialect string

const (
	DialectSQLite  Dialect = "sqlite"
	DialectPostgres Dialect = "postgres"
)

// Open opens a database connection and runs migrations.
// driver: "sqlite" or "postgres"
// dsn: connection string (file path for sqlite, URL for postgres)
func Open(driver, dsn string) (*sql.DB, error) {
	dialect := Dialect(driver)

	switch dialect {
	case DialectSQLite:
		return openSQLite(dsn)
	case DialectPostgres:
		return openPostgres(dsn)
	default:
		return nil, fmt.Errorf("unsupported database driver: %s", driver)
	}
}

func openSQLite(path string) (*sql.DB, error) {
	dir := filepath.Dir(path)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create db dir: %w", err)
		}
	}
	dsn := fmt.Sprintf("file:%s?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)", filepath.ToSlash(path))
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetConnMaxLifetime(0)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := migrate(db, DialectSQLite); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func openPostgres(dsn string) (*sql.DB, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(5 * time.Minute)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := migrate(db, DialectPostgres); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

// migrate runs all pending migrations for the given dialect.
func migrate(db *sql.DB, dialect Dialect) error {
	// Create migrations tracking table
	createMigrationsTable := `CREATE TABLE IF NOT EXISTS schema_migrations (
		name TEXT PRIMARY KEY,
		applied_at INTEGER NOT NULL
	)`
	if dialect == DialectPostgres {
		createMigrationsTable = `CREATE TABLE IF NOT EXISTS schema_migrations (
			name TEXT PRIMARY KEY,
			applied_at BIGINT NOT NULL
		)`
	}
	if _, err := db.Exec(createMigrationsTable); err != nil {
		return err
	}

	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		return err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	for _, name := range names {
		var exists int
		checkQuery := `SELECT COUNT(1) FROM schema_migrations WHERE name=?`
		if dialect == DialectPostgres {
			checkQuery = `SELECT COUNT(1) FROM schema_migrations WHERE name=$1`
		}
		if err := db.QueryRow(checkQuery, name).Scan(&exists); err != nil {
			return err
		}
		if exists > 0 {
			continue
		}

		body, err := migrationFS.ReadFile("migrations/" + name)
		if err != nil {
			return err
		}

		// Adapt SQL for dialect
		sql := adaptSQL(string(body), dialect)

		tx, err := db.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(sql); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("migration %s: %w", name, err)
		}

		insertQuery := `INSERT INTO schema_migrations(name, applied_at) VALUES(?,?)`
		if dialect == DialectPostgres {
			insertQuery = `INSERT INTO schema_migrations(name, applied_at) VALUES($1,$2)`
		}
		if _, err := tx.Exec(insertQuery, name, time.Now().UnixMilli()); err != nil {
			_ = tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// adaptSQL converts SQL syntax for the target dialect.
func adaptSQL(sql string, dialect Dialect) string {
	if dialect == DialectSQLite {
		return sql
	}

	// Postgres adaptations
	result := sql

	// Remove SQLite-specific PRAGMA statements
	result = regexp.MustCompile(`(?i)PRAGMA\s+\w+\s*=\s*\w+;?`).ReplaceAllString(result, "")

	// Convert INTEGER PRIMARY KEY AUTOINCREMENT to BIGSERIAL PRIMARY KEY
	result = regexp.MustCompile(`(?i)\bINTEGER\s+PRIMARY\s+KEY\s+AUTOINCREMENT\b`).
		ReplaceAllString(result, "BIGSERIAL PRIMARY KEY")

	// Convert AUTOINCREMENT alone (for other cases)
	result = regexp.MustCompile(`(?i)\bAUTOINCREMENT\b`).ReplaceAllString(result, "")

	// Convert boolean literals (SQLite uses 0/1, Postgres uses false/true)
	// This is a simplified conversion; complex cases may need manual handling
	// result = regexp.MustCompile(`\bDEFAULT\s+1\b`).ReplaceAllString(result, "DEFAULT true")
	// result = regexp.MustCompile(`\bDEFAULT\s+0\b`).ReplaceAllString(result, "DEFAULT false")

	// Convert ? placeholders to $1, $2, etc.
	paramIndex := 1
	result = regexp.MustCompile(`\?`).ReplaceAllStringFunc(result, func(match string) string {
		placeholder := fmt.Sprintf("$%d", paramIndex)
		paramIndex++
		return placeholder
	})

	return result
}

func NowMS() int64 { return time.Now().UnixMilli() }
