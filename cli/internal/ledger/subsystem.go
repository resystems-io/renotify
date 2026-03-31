package ledger

import (
	"context"
	"log/slog"

	"github.com/nats-io/nats.go"
)

// Subsystem wraps the ledger DB as a daemon.Subsystem so its
// lifecycle is managed by the daemon controller. Register it
// before subsystems that write to the database.
type Subsystem struct {
	dbPath string
	db     *DB
	logger *slog.Logger
}

// NewSubsystem creates a ledger subsystem that will open the
// database at dbPath on Start.
func NewSubsystem(dbPath string, logger *slog.Logger) *Subsystem {
	return &Subsystem{dbPath: dbPath, logger: logger}
}

// Name returns the subsystem's human-readable name.
func (s *Subsystem) Name() string { return "ledger" }

// Start opens the SQLite database and applies migrations. The
// nc parameter is ignored — the ledger has no NATS dependency.
func (s *Subsystem) Start(
	_ context.Context,
	_ *nats.Conn,
	ready chan<- error,
) error {
	db, err := Open(s.dbPath)
	if err != nil {
		if ready != nil {
			ready <- err
			close(ready)
		}
		return err
	}
	s.db = db
	s.logger.Info("ledger opened", "path", s.dbPath)

	if ready != nil {
		close(ready)
	}
	return nil
}

// Stop closes the database connection.
func (s *Subsystem) Stop(_ context.Context) error {
	if s.db == nil {
		return nil
	}
	s.logger.Info("ledger closing")
	return s.db.Close()
}

// DB returns the open database handle. Must only be called after
// Start has signalled readiness.
func (s *Subsystem) DB() *DB {
	return s.db
}
