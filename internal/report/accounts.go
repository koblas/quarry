package report

import (
	"context"

	"github.com/koblas/quarry/internal/store"
)

// Accounts lists the store's accounts in the store's order with their
// balances; closed accounts are left out unless includeClosed is set.
// Store errors are returned unchanged.
func (s *Server) Accounts(ctx context.Context, includeClosed bool) (store.AccountList, error) {
	list, err := s.store.Accounts(ctx)
	if err != nil {
		return store.AccountList{}, err //nolint:wrapcheck // the store's error is final user copy; a prefix would change it
	}
	if includeClosed {
		return list, nil
	}

	open := list.Accounts[:0:0]
	for _, a := range list.Accounts {
		if !a.Closed {
			open = append(open, a)
		}
	}
	list.Accounts = open
	return list, nil
}
