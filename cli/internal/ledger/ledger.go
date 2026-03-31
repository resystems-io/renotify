// Package ledger implements the daemon's persistent SQLite
// database for notification history, active flows, lifecycle
// events, and interjections. See docs/analysis-sqlite-ledger.md.
package ledger

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite" // SQLite driver (pure Go, CGo-free)
)

// DB wraps a SQLite database connection for the ledger.
type DB struct {
	db *sql.DB
}

// Open opens (or creates) the SQLite database at path, sets
// operational PRAGMAs, and applies any pending schema migrations.
func Open(path string) (*DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, fmt.Errorf("ledger: create dir: %w", err)
	}

	sqlDB, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("ledger: open: %w", err)
	}

	// Operational PRAGMAs: WAL for concurrent reads during
	// writes, foreign keys for referential integrity, busy
	// timeout to avoid SQLITE_BUSY under contention.
	for _, pragma := range []string{
		"PRAGMA journal_mode = WAL",
		"PRAGMA foreign_keys = ON",
		"PRAGMA busy_timeout = 5000",
	} {
		if _, err := sqlDB.Exec(pragma); err != nil {
			sqlDB.Close()
			return nil, fmt.Errorf("ledger: %s: %w", pragma, err)
		}
	}

	d := &DB{db: sqlDB}
	if err := d.migrate(); err != nil {
		sqlDB.Close()
		return nil, err
	}
	return d, nil
}

// Close closes the underlying database connection.
func (d *DB) Close() error {
	return d.db.Close()
}

// migrate applies schema migrations based on PRAGMA user_version.
func (d *DB) migrate() error {
	var version int
	if err := d.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return fmt.Errorf("ledger: read schema version: %w", err)
	}

	if version < 1 {
		tx, err := d.db.Begin()
		if err != nil {
			return fmt.Errorf("ledger: begin migration: %w", err)
		}
		if _, err := tx.Exec(schemaV1); err != nil {
			tx.Rollback()
			return fmt.Errorf("ledger: apply schema v1: %w", err)
		}
		if _, err := tx.Exec("PRAGMA user_version = 1"); err != nil {
			tx.Rollback()
			return fmt.Errorf("ledger: set user_version: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("ledger: commit migration: %w", err)
		}
	}

	return nil
}
