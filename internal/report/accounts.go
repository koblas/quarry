package report

import (
	"context"

	"github.com/koblas/quarry/internal/store"
)

// AccountListing is the accounts a listing shows, plus how many closed
// accounts it left out.
type AccountListing struct {
	store.AccountList

	Hidden int
}

// AllHidden reports whether the listing is empty only because every account
// in the store is closed and was left out.
func (l AccountListing) AllHidden() bool {
	return l.Hidden > 0 && len(l.Accounts) == 0
}

// Accounts lists the store's accounts in the store's order with their
// balances; closed accounts are left out, and counted in Hidden, unless
// includeClosed is set. Store errors are returned unchanged.
func (s *Server) Accounts(ctx context.Context, includeClosed bool) (AccountListing, error) {
	list, err := s.store.Accounts(ctx)
	if err != nil {
		return AccountListing{}, err //nolint:wrapcheck // the store's error is final user copy; a prefix would change it
	}
	if includeClosed {
		return AccountListing{AccountList: list}, nil
	}

	open := list.Accounts[:0:0]
	for _, a := range list.Accounts {
		if !a.Closed {
			open = append(open, a)
		}
	}
	hidden := len(list.Accounts) - len(open)
	list.Accounts = open
	return AccountListing{AccountList: list, Hidden: hidden}, nil
}
