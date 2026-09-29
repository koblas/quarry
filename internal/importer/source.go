package importer

import (
	"context"
	"fmt"

	"github.com/koblas/quarry/internal/platform/sqlite"
)

var _ Source = (*sqlite.DB)(nil)

// defaultOpener opens path read-only through platform/sqlite.
func defaultOpener(ctx context.Context, path string) (Source, error) {
	db, err := sqlite.OpenReadOnly(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("open source: %w", err)
	}
	return db, nil
}
