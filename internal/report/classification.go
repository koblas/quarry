package report

import (
	"slices"

	"github.com/koblas/quarry/internal/store"
)

// Classification is the account ids the config lists as registered and as non-registered.
// An id in both lists is refused at config load, so Of reads it as registered only for a value built by hand.
type Classification struct {
	Registered, NonRegistered []string
}

// Of reports whether a is registered: true when listed registered, false when listed
// non-registered, nil when listed in neither.
func (c Classification) Of(a store.Account) *bool {
	if slices.Contains(c.Registered, a.ID) {
		return new(true)
	}
	if slices.Contains(c.NonRegistered, a.ID) {
		return new(false)
	}

	return nil
}

// UnmatchedAccounts is the ids each account list names that no account in quarry's store has, in file order with repeats.
type UnmatchedAccounts struct {
	Registered, NonRegistered []string
}

// Unmatched is the ids c lists that name none of accounts, whatever the account's type or whether it is closed.
func (c Classification) Unmatched(accounts []store.Account) UnmatchedAccounts {
	known := make(map[string]bool, len(accounts))
	for _, a := range accounts {
		known[a.ID] = true
	}
	listed := func(id string) bool { return known[id] }

	return UnmatchedAccounts{
		Registered:    slices.DeleteFunc(slices.Clone(c.Registered), listed),
		NonRegistered: slices.DeleteFunc(slices.Clone(c.NonRegistered), listed),
	}
}

// Unclassified reports whether a is an investment account listed in neither list.
func (c Classification) Unclassified(a store.Account) bool {
	return store.IsInvestmentAccount(a.Type) && c.Of(a) == nil
}

// countUnclassified is how many of accounts are investment accounts listed in neither list, closed ones included.
func (c Classification) countUnclassified(accounts []store.Account) int {
	n := 0
	for _, a := range accounts {
		if c.Unclassified(a) {
			n++
		}
	}

	return n
}
