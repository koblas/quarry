package report

import (
	"slices"

	"github.com/koblas/quarry/internal/store"
)

// Classification is the account ids the config lists as registered and as non-registered.
// An id in both lists counts as registered.
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

// Unclassified reports whether a is an investment account listed in neither list.
func (c Classification) Unclassified(a store.Account) bool {
	return store.IsInvestmentAccount(a.Type) && c.Of(a) == nil
}
