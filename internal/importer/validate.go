package importer

import (
	"cmp"
	"slices"
	"strings"

	"github.com/koblas/quarry/internal/store"
)

// validate runs every check on rows against statements (keyed by account
// id) and returns all their results, even once one has already failed.
func validate(rows store.Rows, statements map[string]parsedStatement) store.Validation {
	return store.Validation{
		Balances: checkBalances(rows, statements),
		Splits:   checkSplits(rows),
	}
}

// checkBalances compares reconciled sums against statements; an account
// missing from statements is never reconciled, not mismatched.
// NeverReconciled and Mismatched are sorted by account name (byte order)
// then account source id, the display order for both V1 and --json.
func checkBalances(rows store.Rows, statements map[string]parsedStatement) store.BalanceCheck {
	reconciledSums := make(map[string]int64, len(rows.Accounts))
	for _, txn := range rows.Transactions {
		if txn.Status == "reconciled" {
			reconciledSums[txn.AccountID] += txn.Amount
		}
	}

	var check store.BalanceCheck
	for _, acct := range rows.Accounts {
		if investmentTypes[acct.Type] {
			check.InvestmentAccounts++
			continue
		}
		stmt, ok := statements[acct.ID]
		if !ok {
			check.NeverReconciled = append(check.NeverReconciled, acct)
			continue
		}
		check.Checked++
		sum := reconciledSums[acct.ID]
		if sum != stmt.Cents {
			check.Mismatched = append(check.Mismatched, store.BalanceMismatch{
				ID: acct.ID, Name: acct.Name, SourceID: acct.SourceID, Currency: acct.Currency, Closed: acct.Closed, Active: acct.Active,
				StatementDate: stmt.Date, Quarry: sum, Quicken: stmt.Cents, Difference: sum - stmt.Cents,
			})
		}
	}
	slices.SortFunc(check.NeverReconciled, byNameThenSourceID(func(a store.Account) (string, int64) { return a.Name, a.SourceID }))
	slices.SortFunc(check.Mismatched, byNameThenSourceID(func(m store.BalanceMismatch) (string, int64) { return m.Name, m.SourceID }))
	return check
}

// byNameThenSourceID builds a slices.SortFunc comparator ordering by name
// (byte order) then numeric source id, the tie-break a string comparison of
// two-digit and one-digit ids would get wrong.
func byNameThenSourceID[T any](key func(T) (string, int64)) func(T, T) int {
	return func(a, b T) int {
		aName, aID := key(a)
		bName, bID := key(b)
		if c := strings.Compare(aName, bName); c != 0 {
			return c
		}
		return cmp.Compare(aID, bID)
	}
}

// checkSplits verifies every imported transaction has at least one split
// and its splits sum to its amount. Mismatched is sorted by date, account
// name (byte order), account source id, then transaction source id — the
// display order for both V1 and --json.
func checkSplits(rows store.Rows) store.SplitCheck {
	sums := make(map[string]int64, len(rows.Transactions))
	counts := make(map[string]int, len(rows.Transactions))
	for _, s := range rows.Splits {
		sums[s.TransactionID] += s.Amount
		counts[s.TransactionID]++
	}
	accountNames := make(map[string]string, len(rows.Accounts))
	accountSourceIDs := make(map[string]int64, len(rows.Accounts))
	for _, a := range rows.Accounts {
		accountNames[a.ID] = a.Name
		accountSourceIDs[a.ID] = a.SourceID
	}
	payeeNames := make(map[string]string, len(rows.Payees))
	for _, p := range rows.Payees {
		payeeNames[p.ID] = p.Name
	}

	check := store.SplitCheck{Checked: len(rows.Transactions)}
	var mismatched []store.Transaction
	for _, txn := range rows.Transactions {
		if counts[txn.ID] > 0 && sums[txn.ID] == txn.Amount {
			continue
		}
		mismatched = append(mismatched, txn)
	}
	slices.SortFunc(mismatched, func(a, b store.Transaction) int {
		if c := a.Date.Compare(b.Date); c != 0 {
			return c
		}
		if c := strings.Compare(accountNames[a.AccountID], accountNames[b.AccountID]); c != 0 {
			return c
		}
		if c := cmp.Compare(accountSourceIDs[a.AccountID], accountSourceIDs[b.AccountID]); c != 0 {
			return c
		}
		return cmp.Compare(a.SourceID, b.SourceID)
	})

	for _, txn := range mismatched {
		var payee string
		if txn.PayeeID != nil {
			payee = payeeNames[*txn.PayeeID]
		}
		check.Mismatched = append(check.Mismatched, store.SplitMismatch{
			ID: txn.ID, SourceID: txn.SourceID, Date: txn.Date, Account: accountNames[txn.AccountID], Currency: txn.Currency,
			Payee: payee, Amount: txn.Amount, SplitsTotal: sums[txn.ID],
		})
	}
	return check
}
