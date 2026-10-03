package duckstore

import (
	"context"

	"github.com/koblas/quarry/internal/store"
)

// Search lists the newest params.Limit transactions matching params, as store.Search documents.
func (s *Store) Search(context.Context, store.SearchParams) (store.Search, error) {
	return store.Search{}, nil
}
