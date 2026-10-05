package report

import "github.com/koblas/quarry/internal/store"

// Classification is the account ids the config lists as registered and as non-registered.
type Classification struct {
	Registered, NonRegistered []string
}

// Of reports whether a is registered: true when listed registered, false when listed non-registered, nil when listed in neither.
func (c Classification) Of(a store.Account) *bool {
	return nil
}

// Unclassified reports whether a is an investment account listed in neither list.
func (c Classification) Unclassified(a store.Account) bool {
	return false
}
