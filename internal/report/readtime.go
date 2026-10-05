package report

import (
	"slices"

	"github.com/koblas/quarry/internal/finding"
	"github.com/koblas/quarry/internal/store"
)

// readTimeFindings is list with the findings computed from the config at read time added to the stored ones.
// It is the one place such findings enter a listing, so every site that classifies findings sees the same set.
func readTimeFindings(list store.FindingList, c Classification) store.FindingList {
	list.Findings = slices.Concat(list.Findings, unclassifiedFindings(list.Accounts, c))
	return list
}

// ReadTimeStates is the state of each finding c computes from list's accounts, none new. Findings
// stored in list are left out, so a caller holding the stored states adds them without counting twice.
func ReadTimeStates(list store.FindingList, c Classification) []finding.State {
	list.Findings = nil
	_, states := knownFindings(readTimeFindings(list, c))
	return states
}

// unclassifiedFindings is one open finding per account c leaves unclassified, with no first-found time.
func unclassifiedFindings(accounts []store.Account, c Classification) []store.Finding {
	var found []store.Finding
	for _, a := range accounts {
		if !c.Unclassified(a) {
			continue
		}
		found = append(found, store.Finding{
			ID:   finding.ID(finding.UnclassifiedAccount, a.ID),
			Type: finding.UnclassifiedAccount,
			Items: []store.FindingItem{{
				AccountID: a.ID, Account: a.Name, AccountType: a.Type, Currency: a.Currency, Closed: a.Closed,
			}},
		})
	}
	return found
}
