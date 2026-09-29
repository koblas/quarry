package snapshot

import (
	"context"
	"fmt"
	"time"

	"github.com/koblas/quarry/internal/platform/sqlite"
)

// sqliteSource is the production Source adapter: a single read-only
// connection opened over platform/sqlite.
type sqliteSource struct {
	db *sqlite.DB
	// busyTimeout governs both Open's connection-level busy_timeout and
	// Backup's online-backup retry deadline.
	busyTimeout time.Duration
}

var _ Source = (*sqliteSource)(nil)

// newSQLiteSource returns the production Source adapter.
func newSQLiteSource(busyTimeout time.Duration) Source {
	return &sqliteSource{busyTimeout: busyTimeout}
}

func (s *sqliteSource) Open(ctx context.Context, path string) error {
	db, err := sqlite.OpenReadOnlyBusy(ctx, path, s.busyTimeout)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	s.db = db
	return nil
}

func (s *sqliteSource) Probe(ctx context.Context) error {
	if _, err := s.db.QueryInt(ctx, "SELECT count(*) FROM sqlite_master"); err != nil {
		// unreachable: Open already runs this identical probe query as part of sqlite.OpenReadOnly; it fails only if the connection breaks between the two calls.
		return fmt.Errorf("probe: %w", err)
	}
	return nil
}

func (s *sqliteSource) Backup(ctx context.Context, destPath string) error {
	if err := sqlite.Backup(ctx, s.db, destPath, s.busyTimeout); err != nil {
		return fmt.Errorf("backup: %w", err)
	}
	return nil
}

func (s *sqliteSource) Close() error {
	if s.db == nil {
		return nil
	}
	if err := s.db.Close(); err != nil {
		// unreachable: database/sql.DB.Close of a read-only connection with no open rows returns only sqlite3_close_v2's result, which is SQLITE_OK for a live connection with no unfinalized statements.
		return fmt.Errorf("close source: %w", err)
	}
	return nil
}
