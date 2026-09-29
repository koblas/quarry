package report

import (
	"context"

	"github.com/koblas/quarry/internal/store"
)

// Store is the read side of quarry's store: each method opens the store
// read-only, answers, and closes it again.
type Store interface {
	// Status describes the store and the import run that built it.
	Status(ctx context.Context) (store.Status, error)
}
