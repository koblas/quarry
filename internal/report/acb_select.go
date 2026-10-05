package report

import (
	"cmp"
	"maps"
	"slices"
	"strings"

	"github.com/koblas/quarry/internal/store"
)

// withSelection is a with the securities req.Securities name recorded on it. It refuses with the first selector,
// in request order, that matches no security a covers: one with a pool event or a registered-account transaction.
func (a ACB) withSelection(history store.InvestmentHistory, req ACBRequest) (ACB, error) {
	pooled := make(map[string]bool, len(a.Securities))
	for _, position := range a.Securities {
		pooled[position.Security.ID] = true
	}
	registered := registeredHoldings(history, req)

	picked := make(map[string]store.Security)
	for _, selector := range req.Securities {
		covered := slices.DeleteFunc(matchSecurities(history.Securities, selector), func(s store.Security) bool {
			return !pooled[s.ID] && !registered[s.ID]
		})
		if len(covered) == 0 {
			return ACB{}, unknownSecurityRefusal(selector)
		}
		for _, s := range covered {
			picked[s.ID] = s
		}
	}

	ordered := slices.SortedFunc(maps.Values(picked), func(x, y store.Security) int {
		return cmp.Or(
			cmp.Compare(strings.ToLower(x.Name), strings.ToLower(y.Name)),
			cmp.Compare(x.ID, y.ID),
		)
	})
	a.Selected = true
	for _, s := range ordered {
		a.SelectedIDs = append(a.SelectedIDs, s.ID)
		if !pooled[s.ID] {
			a.RegisteredOnly = append(a.RegisteredOnly, s)
		}
	}

	return a, nil
}

// matchSecurities is the securities selector names: the one with that id alone, else every one whose ticker or
// name equals it ignoring case. An empty selector names none.
func matchSecurities(securities []store.Security, selector string) []store.Security {
	if selector == "" {
		return nil
	}
	if i := slices.IndexFunc(securities, func(s store.Security) bool { return s.ID == selector }); i >= 0 {
		return []store.Security{securities[i]}
	}

	return slices.DeleteFunc(slices.Clone(securities), func(s store.Security) bool {
		tickerMatches := s.Ticker != nil && strings.EqualFold(*s.Ticker, selector)
		return !tickerMatches && !strings.EqualFold(s.Name, selector)
	})
}

// registeredHoldings is the ids of the securities with a transaction through req.Today in a registered account.
func registeredHoldings(history store.InvestmentHistory, req ACBRequest) map[string]bool {
	inRegistered := accountsClassified(history.Accounts, req.Classification, true)
	held := make(map[string]bool)
	for _, tx := range history.Transactions {
		// unreachable: the nil-security arm, since duckstore's InvestmentHistory query selects only rows WHERE security_id IS NOT NULL (internal/store/duckstore/investments.go:20).
		if tx.SecurityID == nil {
			continue
		}
		if inRegistered[tx.AccountID] && !tx.Date.After(req.Today) {
			held[*tx.SecurityID] = true
		}
	}

	return held
}
