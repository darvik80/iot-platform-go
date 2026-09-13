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
	DialectSQLite   Dialect = "sqlite"
	DialectPostgres Dialect = "postgres"
)

// ── DB wrapper ──────────────────────────────────────────────────────────────
// DB wraps *sql.DB to auto-convert ? placeholders to $N for PostgreSQL and
// to make LastInsertId() work via RETURNING id for INSERT statements.

// DB is a dialect-aware wrapper around *sql.DB.
type DB struct {
	Raw     *sql.DB
	Dialect Dialect
}

// Open opens a database connection and runs migrations.
// driver: "sqlite" or "postgres"
// dsn: connection string (file path for sqlite, URL for postgres)
func Open(driver, dsn string) (*DB, error) {
	dialect := Dialect(driver)

	var raw *sql.DB
	var err error

	switch dialect {
	case DialectSQLite:
		raw, err = openSQLite(dsn)
	case DialectPostgres:
		raw, err = openPostgres(dsn)
	default:
		return nil, fmt.Errorf("unsupported database driver: %s", driver)
	}
	if err != nil {
		return nil, err
	}
	return &DB{Raw: raw, Dialect: dialect}, nil
}

// Close closes the underlying database.
func (d *DB) Close() error { return d.Raw.Close() }

// Ping checks the database connection.
func (d *DB) Ping() error { return d.Raw.Ping() }

// SetMaxOpenConns sets the max open connections on the underlying DB.
func (d *DB) SetMaxOpenConns(n int) { d.Raw.SetMaxOpenConns(n) }

// SetMaxIdleConns sets the max idle connections on the underlying DB.
func (d *DB) SetMaxIdleConns(n int) { d.Raw.SetMaxIdleConns(n) }

// SetConnMaxLifetime sets the max connection lifetime on the underlying DB.
func (d *DB) SetConnMaxLifetime(t time.Duration) { d.Raw.SetConnMaxLifetime(t) }

// Exec executes a query with automatic placeholder conversion.
// For PostgreSQL INSERT statements without RETURNING, automatically appends
// RETURNING id so that LastInsertId() works correctly.
func (d *DB) Exec(query string, args ...any) (Result, error) {
	q, a := d.convert(query, args...)
	if d.Dialect == DialectPostgres && isInsert(q) && !hasReturning(q) {
		return d.execInsertReturning(q, a)
	}
	res, err := d.Raw.Exec(q, a...)
	return Result{inner: res, pg: d.Dialect == DialectPostgres}, err
}

// Query executes a query that returns rows with automatic placeholder conversion.
func (d *DB) Query(query string, args ...any) (*sql.Rows, error) {
	q, a := d.convert(query, args...)
	return d.Raw.Query(q, a...)
}

// QueryRow executes a query expected to return a single row.
func (d *DB) QueryRow(query string, args ...any) *sql.Row {
	q, a := d.convert(query, args...)
	return d.Raw.QueryRow(q, a...)
}

// Begin starts a transaction with dialect-aware SQL conversion.
func (d *DB) Begin() (*Tx, error) {
	tx, err := d.Raw.Begin()
	if err != nil {
		return nil, err
	}
	return &Tx{inner: tx, dialect: d.Dialect}, nil
}

// convert applies placeholder conversion for the current dialect.
func (d *DB) convert(query string, args ...any) (string, []any) {
	if d.Dialect == DialectPostgres {
		return convertPlaceholders(query), args
	}
	return query, args
}

// execInsertReturning handles INSERT ... RETURNING id for PostgreSQL.
func (d *DB) execInsertReturning(query string, args []any) (Result, error) {
	q := strings.TrimRight(query, "; \t\n") + " RETURNING id"
	var id int64
	err := d.Raw.QueryRow(q, args...).Scan(&id)
	if err != nil {
		return Result{pg: true}, err
	}
	return Result{pg: true, lastID: id, hasID: true}, nil
}

// ── Tx wrapper ──────────────────────────────────────────────────────────────

// Tx is a dialect-aware wrapper around *sql.Tx.
type Tx struct {
	inner   *sql.Tx
	dialect Dialect
}

// Exec executes a query within the transaction.
func (t *Tx) Exec(query string, args ...any) (Result, error) {
	q, a := t.convert(query, args...)
	if t.dialect == DialectPostgres && isInsert(q) && !hasReturning(q) {
		return t.execInsertReturning(q, a)
	}
	res, err := t.inner.Exec(q, a...)
	return Result{inner: res, pg: t.dialect == DialectPostgres}, err
}

// Query executes a query that returns rows within the transaction.
func (t *Tx) Query(query string, args ...any) (*sql.Rows, error) {
	q, a := t.convert(query, args...)
	return t.inner.Query(q, a...)
}

// QueryRow executes a query expected to return a single row within the transaction.
func (t *Tx) QueryRow(query string, args ...any) *sql.Row {
	q, a := t.convert(query, args...)
	return t.inner.QueryRow(q, a...)
}

// Commit commits the transaction.
func (t *Tx) Commit() error { return t.inner.Commit() }

// Rollback aborts the transaction.
func (t *Tx) Rollback() error { return t.inner.Rollback() }

func (t *Tx) convert(query string, args ...any) (string, []any) {
	if t.dialect == DialectPostgres {
		return convertPlaceholders(query), args
	}
	return query, args
}

func (t *Tx) execInsertReturning(query string, args []any) (Result, error) {
	q := strings.TrimRight(query, "; \t\n") + " RETURNING id"
	var id int64
	err := t.inner.QueryRow(q, args...).Scan(&id)
	if err != nil {
		return Result{pg: true}, err
	}
	return Result{pg: true, lastID: id, hasID: true}, nil
}

// ── Result wrapper ──────────────────────────────────────────────────────────

// Result wraps sql.Result to provide LastInsertId for PostgreSQL.
type Result struct {
	inner  sql.Result
	pg     bool
	lastID int64
	hasID  bool
}

// LastInsertId returns the auto-generated ID (works for both SQLite and PostgreSQL).
func (r Result) LastInsertId() (int64, error) {
	if r.hasID {
		return r.lastID, nil
	}
	if r.inner == nil {
		return 0, fmt.Errorf("no result")
	}
	return r.inner.LastInsertId()
}

// RowsAffected returns the number of affected rows.
func (r Result) RowsAffected() (int64, error) {
	if r.inner == nil {
		return 0, fmt.Errorf("no result")
	}
	return r.inner.RowsAffected()
}

// ── Placeholder conversion ──────────────────────────────────────────────────

var placeholderRe = regexp.MustCompile(`\?`)

// convertPlaceholders converts ? placeholders to $1, $2, … for PostgreSQL.
func convertPlaceholders(query string) string {
	n := 1
	return placeholderRe.ReplaceAllStringFunc(query, func(string) string {
		s := fmt.Sprintf("$%d", n)
		n++
		return s
	})
}

var insertRe = regexp.MustCompile(`(?i)^\s*INSERT\s+INTO\b`)

func isInsert(q string) bool { return insertRe.MatchString(q) }

func hasReturning(q string) bool {
	return strings.Contains(strings.ToUpper(q), "RETURNING")
}

// ── Raw open helpers (return *sql.DB for migration use) ─────────────────────

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

// ── Migration engine (uses raw *sql.DB, handles its own placeholders) ───────

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

// adaptSQL converts migration SQL syntax for the target dialect.
func adaptSQL(sql string, dialect Dialect) string {
	if dialect == DialectSQLite {
		return sql
	}

	result := sql

	// Remove SQLite-specific PRAGMA statements
	result = regexp.MustCompile(`(?i)PRAGMA\s+\w+\s*=\s*\w+;?`).ReplaceAllString(result, "")

	// Convert INSERT OR IGNORE INTO ... ; → INSERT INTO ... ON CONFLICT DO NOTHING;
	result = convertInsertOrIgnore(result)

	// Convert INTEGER PRIMARY KEY AUTOINCREMENT to BIGSERIAL PRIMARY KEY
	result = regexp.MustCompile(`(?i)\bINTEGER\s+PRIMARY\s+KEY\s+AUTOINCREMENT\b`).
		ReplaceAllString(result, "BIGSERIAL PRIMARY KEY")

	// Convert AUTOINCREMENT alone (for other cases)
	result = regexp.MustCompile(`(?i)\bAUTOINCREMENT\b`).ReplaceAllString(result, "")

	// Convert remaining INTEGER to BIGINT for PostgreSQL (timestamps need 64-bit)
	// This must be done after PRIMARY KEY AUTOINCREMENT conversion
	// Use negative lookahead to avoid converting INTEGER PRIMARY KEY
	result = regexp.MustCompile(`(?i)\bINTEGER\b(?!\s+PRIMARY\s+KEY)`).ReplaceAllString(result, "BIGINT")

	// Convert ? placeholders to $1, $2, etc.
	paramIndex := 1
	result = regexp.MustCompile(`\?`).ReplaceAllStringFunc(result, func(match string) string {
		placeholder := fmt.Sprintf("$%d", paramIndex)
		paramIndex++
		return placeholder
	})

	return result
}

// convertInsertOrIgnore converts SQLite "INSERT OR IGNORE INTO ... ;" to
// PostgreSQL "INSERT INTO ... ON CONFLICT DO NOTHING;".
func convertInsertOrIgnore(sql string) string {
	re := regexp.MustCompile(`(?i)\bINSERT\s+OR\s+IGNORE\s+INTO\b`)
	loc := re.FindAllStringIndex(sql, -1)
	if len(loc) == 0 {
		return sql
	}

	result := re.ReplaceAllString(sql, "INSERT INTO")

	for i := len(loc) - 1; i >= 0; i-- {
		start := loc[i][0]
		semiIdx := strings.Index(result[start:], ";")
		if semiIdx >= 0 {
			absIdx := start + semiIdx
			result = result[:absIdx] + " ON CONFLICT DO NOTHING" + result[absIdx:]
		} else {
			result += " ON CONFLICT DO NOTHING"
		}
	}
	return result
}

func NowMS() int64 { return time.Now().UnixMilli() }
