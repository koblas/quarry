package report

import (
	"slices"

	"github.com/koblas/quarry/internal/finding"
	"github.com/koblas/quarry/internal/store"
)

// keyKind says what a group key's value is, so a payee_id fallback never merges with an equal payee key.
type keyKind int

const (
	// keyByPayeeKey is a value made by finding.PayeeKey from the payee's name.
	keyByPayeeKey keyKind = iota
	// keyByPayeeID is the exact payee_id, used when the name leaves no payee key.
	keyByPayeeID
)

// groupKey is what charges of one series share: the payee (by key, or by id when the key is
// empty) and the currency. Account and category are not in it.
type groupKey struct {
	kind     keyKind
	value    string
	currency string
}

// chargeGroup is the charges sharing key, oldest first.
type chargeGroup struct {
	key     groupKey
	charges []store.Charge
}

// chargeKey is the group key of c; ok is false for a charge with no payee, which belongs to no group.
func chargeKey(c store.Charge) (groupKey, bool) {
	if c.Payee == nil || c.PayeeID == nil {
		return groupKey{}, false
	}
	if payeeKey := finding.PayeeKey(*c.Payee); payeeKey != "" {
		return groupKey{kind: keyByPayeeKey, value: payeeKey, currency: c.Currency}, true
	}
	return groupKey{kind: keyByPayeeID, value: *c.PayeeID, currency: c.Currency}, true
}

// groupCharges splits charges, already ordered by date then source id, into groups in order of
// each group's first charge; a charge with no payee is left out.
func groupCharges(charges []store.Charge) []chargeGroup {
	var groups []chargeGroup
	index := map[groupKey]int{}
	for _, c := range charges {
		key, ok := chargeKey(c)
		if !ok {
			continue
		}
		i, seen := index[key]
		if !seen {
			i = len(groups)
			index[key] = i
			groups = append(groups, chargeGroup{key: key})
		}
		groups[i].charges = append(groups[i].charges, c)
	}
	return groups
}

// identitiesOf is the distinct payees (by id) and accounts (by id) of run, in order of first appearance;
// neither is nil.
func identitiesOf(run []store.Charge) ([]SeriesPayee, []store.Account) {
	payees, accounts := []SeriesPayee{}, []store.Account{}
	for _, c := range run {
		if !slices.ContainsFunc(payees, func(p SeriesPayee) bool { return p.ID == *c.PayeeID }) {
			payees = append(payees, SeriesPayee{ID: *c.PayeeID, Name: *c.Payee})
		}
		if !slices.ContainsFunc(accounts, func(a store.Account) bool { return a.ID == c.Account.ID }) {
			accounts = append(accounts, c.Account)
		}
	}
	return payees, accounts
}
