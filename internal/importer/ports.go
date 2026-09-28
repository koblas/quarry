package importer

import (
	"context"

	"github.com/koblas/quarry/internal/store"
)

// Source is a read-only handle on the snapshot's SQLite database: the
// importer's own multi-row read surface.
type Source interface {
	// QueryRows runs query and calls row once per result row.
	QueryRows(ctx context.Context, query string, args []any, row func(scan func(dest ...any) error) error) error
	// Close closes the underlying connection.
	Close() error
}

// SourceOpener opens path (a snapshot's SQLite file) as a Source.
type SourceOpener func(ctx context.Context, path string) (Source, error)

// Store is the persistence port Import writes mapped rows into. Replace
// builds a new store from rows and swaps it in only once the build
// succeeds, returning the path it wrote to.
type Store interface {
	Replace(ctx context.Context, rows store.Rows) (string, error)
}
