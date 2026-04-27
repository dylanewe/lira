package db

import (
	"database/sql"
	"embed"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
)

//go:embed migrations/*.sql
var migrations embed.FS

// Open opens (or creates) the SQLite database at the given path and runs
// any pending migrations. It returns a ready-to-use *sql.DB.
func Open(dbPath string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}

	db, err := sql.Open("sqlite3", dbPath+"?_foreign_keys=on&_journal_mode=WAL")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}

	if err := runMigrations(db); err != nil {
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	return db, nil
}

func runMigrations(db *sql.DB) error {
	// Create the migrations tracking table if it doesn't exist yet.
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			filename TEXT PRIMARY KEY,
			applied_at TEXT NOT NULL DEFAULT (datetime('now'))
		)
	`)
	if err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	entries, err := migrations.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		filename := entry.Name()

		var exists int
		err := db.QueryRow(
			`SELECT COUNT(*) FROM schema_migrations WHERE filename = ?`, filename,
		).Scan(&exists)
		if err != nil {
			return fmt.Errorf("check migration %s: %w", filename, err)
		}
		if exists > 0 {
			continue
		}

		sql, err := migrations.ReadFile("migrations/" + filename)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", filename, err)
		}

		if _, err := db.Exec(string(sql)); err != nil {
			return fmt.Errorf("apply migration %s: %w", filename, err)
		}

		if _, err := db.Exec(
			`INSERT INTO schema_migrations (filename) VALUES (?)`, filename,
		); err != nil {
			return fmt.Errorf("record migration %s: %w", filename, err)
		}
	}

	return nil
}

// NextTicketID atomically increments the global counter and returns the next
// formatted ticket ID, e.g. "L-0042".
func NextTicketID(db *sql.DB) (string, error) {
	tx, err := db.Begin()
	if err != nil {
		return "", err
	}
	defer tx.Rollback()

	var next int
	err = tx.QueryRow(`UPDATE ticket_counter SET next_val = next_val + 1 WHERE id = 1 RETURNING next_val`).Scan(&next)
	if err != nil {
		return "", fmt.Errorf("increment ticket counter: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return "", err
	}

	return fmt.Sprintf("L-%04d", next), nil
}
