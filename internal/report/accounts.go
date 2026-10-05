package report

import (
	"context"
	"slices"
	"strings"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/store"
)

// AccountListing is the accounts a listing shows, plus how many closed
// accounts it left out.
type AccountListing struct {
	store.AccountList

	Hidden int

	// Currency is the reporting currency the listing was asked for.
	Currency money.Currency
}

// ConvertedBalance is a's balance in cents in the listing's currency; nil in a native listing and
// when no rate on or before AsOf converts it.
func (l AccountListing) ConvertedBalance(a store.AccountBalance) *int64 {
	if l.Currency == money.CAD {
		return a.BalanceCAD
	}
	if l.Currency == money.USD {
		return a.BalanceUSD
	}
	return nil
}

// NeedsRate reports whether a's balance should convert but no rate converts it.
func (l AccountListing) NeedsRate(a store.AccountBalance) bool {
	return l.Currency != money.Native && l.ConvertedBalance(a) == nil
}

// AllHidden reports whether the listing is empty only because every account
// in the store is closed and was left out.
func (l AccountListing) AllHidden() bool {
	return l.Hidden > 0 && len(l.Accounts) == 0
}

// Accounts lists the store's accounts in the store's order with their
// balances in currency; closed accounts are left out, and counted in Hidden,
// unless includeClosed is set. It refuses like Status.
func (s *Server) Accounts(ctx context.Context, includeClosed bool, currency money.Currency) (AccountListing, error) {
	list, err := s.store.Accounts(ctx)
	if err != nil {
		return AccountListing{}, s.readRefusal(ctx, "accounts", err)
	}
	if includeClosed {
		return AccountListing{AccountList: list, Currency: currency}, nil
	}

	open := list.Accounts[:0:0]
	listed := make(map[string]bool, len(list.Accounts))
	for _, a := range list.Accounts {
		if !a.Closed {
			open = append(open, a)
			listed[a.ID] = true
		}
	}
	hidden := len(list.Accounts) - len(open)
	list.Accounts = open
	list.Unvalued = slices.DeleteFunc(slices.Clone(list.Unvalued), func(held store.UnvaluedHolding) bool { return !listed[held.AccountID] })
	return AccountListing{AccountList: list, Hidden: hidden, Currency: currency}, nil
}

// resolveAccounts is the accounts args name, in the order given and without repeats: each arg is an
// account's id, else its name ignoring case. The first arg naming none or several is refused.
func (s *Server) resolveAccounts(ctx context.Context, command string, args []string) ([]store.Account, error) {
	list, err := s.store.Accounts(ctx)
	if err != nil {
		return nil, s.readRefusal(ctx, command, err)
	}
	var resolved []store.Account
	seen := make(map[string]bool, len(args))
	for _, arg := range args {
		account, err := pickAccount(list.Accounts, arg)
		if err != nil {
			return nil, err
		}
		if !seen[account.ID] {
			seen[account.ID] = true
			resolved = append(resolved, account)
		}
	}
	return resolved, nil
}

// namedAccounts is the accounts names resolve to and their ids, both nil when none is named
// (which counts every account, without reading the store's accounts).
func (s *Server) namedAccounts(ctx context.Context, command string, names []string) ([]store.Account, []string, error) {
	if len(names) == 0 {
		return nil, nil, nil
	}
	accounts, err := s.resolveAccounts(ctx, command, names)
	if err != nil {
		return nil, nil, err
	}
	ids := make([]string, len(accounts))
	for i, a := range accounts {
		ids[i] = a.ID
	}
	return accounts, ids, nil
}

// pickAccount is the account arg names: the one with that id, else the only one with that name
// ignoring case. An empty arg names no account, whatever an account's name.
func pickAccount(accounts []store.AccountBalance, arg string) (store.Account, error) {
	if arg == "" {
		return store.Account{}, unknownAccountRefusal(arg)
	}
	for _, a := range accounts {
		if a.ID == arg {
			return a.Account, nil
		}
	}
	var named []store.Account
	for _, a := range accounts {
		if strings.EqualFold(a.Name, arg) {
			named = append(named, a.Account)
		}
	}
	switch len(named) {
	case 0:
		return store.Account{}, unknownAccountRefusal(arg)
	case 1:
		return named[0], nil
	}
	ids := make([]string, len(named))
	for i, a := range named {
		ids[i] = a.ID
	}
	return store.Account{}, ambiguousAccountRefusal(arg, ids)
}
