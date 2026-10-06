package report

import (
	"slices"

	"github.com/koblas/quarry/internal/finding"
	"github.com/koblas/quarry/internal/store"
)

// readTimeFindings is list with the findings computed from the config at read time added to the stored ones.
// It is the one place such findings enter a listing, so every site that classifies findings sees the same set.
func readTimeFindings(list store.FindingList, c Classification) store.FindingList {
	list.Findings = slices.Concat(list.Findings, unclassifiedFindings(list.Accounts, c), sharesWithoutCostFindings(list, c))
	return list
}

// ReadTimeStates is the state of each finding c computes from list's accounts and investments, none new. Findings
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

// sharesWithoutCostFindings is one open finding per add_shares that records no cost, in an account c lists non-registered.
// A registered or unclassified account's adds are none of its business, so an empty c lists none.
func sharesWithoutCostFindings(list store.FindingList, c Classification) []store.Finding {
	accounts := make(map[string]store.Account, len(list.Accounts))
	for _, a := range list.Accounts {
		accounts[a.ID] = a
	}
	names := make(map[string]string, len(list.Investments.Securities))
	for _, s := range list.Investments.Securities {
		names[s.ID] = s.Name
	}

	var found []store.Finding
	for _, tx := range list.Investments.Transactions {
		if tx.Action != store.ActionAddShares || tx.SecurityID == nil || !noCostAcquisition(tx) {
			continue
		}
		a := accounts[tx.AccountID]
		if registered := c.Of(a); registered == nil || *registered {
			continue
		}
		found = append(found, store.Finding{
			ID:   finding.ID(finding.SharesWithoutCost, tx.ID),
			Type: finding.SharesWithoutCost,
			Items: []store.FindingItem{{
				Date: tx.Date, AccountID: a.ID, Account: a.Name, Currency: a.Currency,
				InvestmentTransactionID: &tx.ID, SecurityID: tx.SecurityID, Security: names[*tx.SecurityID], Shares: *tx.Shares,
			}},
		})
	}
	return found
}
