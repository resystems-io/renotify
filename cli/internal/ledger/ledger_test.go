package ledger

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpen_CreatesDB(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if _, err := os.Stat(path); err != nil {
		t.Errorf("database file should exist: %v", err)
	}
}

func TestOpen_MigratesLatest(t *testing.T) {
	db := openTestDB(t)

	var version int
	err := db.db.QueryRow("PRAGMA user_version").Scan(&version)
	if err != nil {
		t.Fatal(err)
	}
	if version != 3 {
		t.Errorf("user_version = %d, want 3", version)
	}
}

func TestOpen_IdempotentMigration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")

	// Open and close twice — second Open should be safe.
	db1, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	db1.Close()

	db2, err := Open(path)
	if err != nil {
		t.Fatalf("second Open should succeed: %v", err)
	}
	defer db2.Close()

	var version int
	db2.db.QueryRow("PRAGMA user_version").Scan(&version)
	if version != 3 {
		t.Errorf("user_version = %d, want 3", version)
	}
}

func TestOpen_WALMode(t *testing.T) {
	db := openTestDB(t)

	var mode string
	err := db.db.QueryRow("PRAGMA journal_mode").Scan(&mode)
	if err != nil {
		t.Fatal(err)
	}
	if mode != "wal" {
		t.Errorf("journal_mode = %q, want %q", mode, "wal")
	}
}

func TestOpen_ForeignKeys(t *testing.T) {
	db := openTestDB(t)

	var fk int
	err := db.db.QueryRow("PRAGMA foreign_keys").Scan(&fk)
	if err != nil {
		t.Fatal(err)
	}
	if fk != 1 {
		t.Errorf("foreign_keys = %d, want 1", fk)
	}
}

func TestClose(t *testing.T) {
	db := openTestDB(t)
	if err := db.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Subsequent query should fail.
	var n int
	err := db.db.QueryRow("SELECT 1").Scan(&n)
	if err == nil {
		t.Error("query after Close should fail")
	}
}

// openTestDB is a test helper that opens a temporary ledger
// database and registers cleanup.
func openTestDB(t *testing.T) *DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}
