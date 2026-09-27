package snapshot

import (
	"context"
	"fmt"

	"github.com/koblas/quarry/internal/platform/sqlite"
)

// sqliteSource is the production Source adapter: a single read-only
// connection opened over platform/sqlite.
type sqliteSource struct {
	db *sqlite.DB
}

var _ Source = (*sqliteSource)(nil)

// newSQLiteSource returns the production Source adapter.
func newSQLiteSource() Source {
	return &sqliteSource{}
}

func (s *sqliteSource) Open(ctx context.Context, path string) error {
	db, err := sqlite.OpenReadOnly(ctx, path)
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
	if err := sqlite.Backup(ctx, s.db, destPath); err != nil {
		return fmt.Errorf("backup: %w", err)
	}
	return nil
}

func (s *sqliteSource) Close() error {
	if s.db == nil {
		return nil
	}
	return s.db.Close()
}
