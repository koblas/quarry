package cli

import "github.com/koblas/quarry/internal/store"

// spendGrouping is how spend presents one --by value: the flag word, the
// column-1 header and the label of the group with no key.
type spendGrouping struct {
	name, header, missing string
}

// spendGroupings holds every --by value spend reads, indexed by grouping.
var spendGroupings = [...]spendGrouping{
	store.SpendByCategory: {name: "category", header: "Category", missing: "(uncategorized)"},
	store.SpendByPayee:    {name: "payee", header: "Payee", missing: "(no payee)"},
	store.SpendByTag:      {name: "tag", header: "Tag", missing: "(no tag)"},
	store.SpendByMonth:    {name: "month", header: "Month"},
}

// errSpendByUnknown refuses a --by that names no grouping spend reads.
var errSpendByUnknown = UsageError{msg: "--by must be category, payee, tag or month"}

// parseSpendGrouping returns the grouping named by a --by value, or
// errSpendByUnknown.
func parseSpendGrouping(name string) (store.SpendingGroup, error) {
	for group, g := range spendGroupings {
		if g.name == name {
			return store.SpendingGroup(group), nil
		}
	}
	return 0, errSpendByUnknown
}
