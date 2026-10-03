package cli

import "github.com/koblas/quarry/internal/store"

// spendGrouping is how spend presents one --by value: the column-1 header and the label of the
// group with no key.
type spendGrouping struct {
	header, missing string
}

// spendGroupings holds every --by value spend reads, indexed by grouping.
var spendGroupings = [...]spendGrouping{
	store.SpendByCategory: {header: "Category", missing: "(uncategorized)"},
	store.SpendByPayee:    {header: "Payee", missing: "(no payee)"},
	store.SpendByTag:      {header: "Tag", missing: "(no tag)"},
	store.SpendByMonth:    {header: "Month"},
}

// errSpendByUnknown refuses a --by that names no grouping spend reads.
var errSpendByUnknown = UsageError{msg: "--by must be category, payee, tag or month"}

// parseSpendGrouping returns the grouping named by a --by value, or errSpendByUnknown.
func parseSpendGrouping(name string) (store.SpendingGroup, error) {
	group, ok := store.ParseSpendingGroup(name)
	if !ok {
		return 0, errSpendByUnknown
	}
	return group, nil
}
