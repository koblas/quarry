package importer

import (
	"context"

	"github.com/koblas/quarry/internal/platform/sqlite"
)

var _ Source = (*sqlite.DB)(nil)

// defaultOpener opens path read-only through platform/sqlite.
func defaultOpener(ctx context.Context, path string) (Source, error) {
	return sqlite.OpenReadOnly(ctx, path)
}
