package document

import "github.com/koblas/quarry/internal/store"

// AccountFilter names one account a report was limited to, as "account_filter" lists it.
type AccountFilter struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// NewAccountFilters is accounts as a report's "account_filter": [] rather than null when none.
func NewAccountFilters(accounts []store.Account) []AccountFilter {
	filter := make([]AccountFilter, len(accounts))
	for i, a := range accounts {
		filter[i] = AccountFilter{ID: a.ID, Name: a.Name}
	}
	return filter
}
