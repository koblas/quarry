package importer

import (
	"cmp"
	"math"
	"slices"
	"strings"

	"github.com/koblas/quarry/internal/store"
)

// validate runs every check on rows against statements (keyed by account
// id) and returns all their results, even once one has already failed.
// transfers is pairTransfers' result, completed here for display.
func validate(rows store.Rows, statements map[string]parsedStatement, transfers store.TransferCheck) store.Validation {
	ix := newRowIndex(rows)
	transfers.OneSided = describeOneSided(ix, rows.Splits, transfers.OneSided)
	return store.Validation{
		Balances:  checkBalances(rows, statements),
		Splits:    checkSplits(ix, rows),
		Transfers: transfers,
	}
}

// rowIndex looks up the accounts, payee names and transactions a check
// reports on, by quarry id.
type rowIndex struct {
	accounts     map[string]store.Account
	payees       map[string]string
	transactions map[string]store.Transaction
}

// newRowIndex indexes rows' accounts, payees and transactions by id.
func newRowIndex(rows store.Rows) rowIndex {
	ix := rowIndex{
		accounts:     make(map[string]store.Account, len(rows.Accounts)),
		payees:       make(map[string]string, len(rows.Payees)),
		transactions: make(map[string]store.Transaction, len(rows.Transactions)),
	}
	for _, a := range rows.Accounts {
		ix.accounts[a.ID] = a
	}
	for _, p := range rows.Payees {
		ix.payees[p.ID] = p.Name
	}
	for _, txn := range rows.Transactions {
		ix.transactions[txn.ID] = txn
	}
	return ix
}

// payee returns txn's payee name, "" when it has none.
func (ix rowIndex) payee(txn store.Transaction) string {
	if txn.PayeeID == nil {
		return ""
	}
	return ix.payees[*txn.PayeeID]
}

// compareTransactions orders a and b by date, account name, account source
// id, then transaction source id, every source id compared numerically.
func (ix rowIndex) compareTransactions(a, b store.Transaction) int {
	if c := a.Date.Compare(b.Date); c != 0 {
		return c
	}
	aAcct, bAcct := ix.accounts[a.AccountID], ix.accounts[b.AccountID]
	if c := strings.Compare(aAcct.Name, bAcct.Name); c != 0 {
		return c
	}
	if c := cmp.Compare(aAcct.SourceID, bAcct.SourceID); c != 0 {
		return c
	}
	return cmp.Compare(a.SourceID, b.SourceID)
}

// describeOneSided fills each one-sided leg's date, account, payee and split
// amount from splits, and sorts the legs by their transaction's order, then
// by split source id.
func describeOneSided(ix rowIndex, splits []store.Split, legs []store.OneSidedTransfer) []store.OneSidedTransfer {
	splitBySourceID := make(map[int64]store.Split, len(splits))
	for _, s := range splits {
		splitBySourceID[s.SourceID] = s
	}
	txnOf := func(leg store.OneSidedTransfer) store.Transaction {
		return ix.transactions[splitBySourceID[leg.SourceID].TransactionID]
	}

	out := slices.Clone(legs)
	for i := range out {
		leg := &out[i]
		txn := txnOf(*leg)
		acct := ix.accounts[txn.AccountID]
		leg.Date, leg.Payee, leg.Amount = txn.Date, ix.payee(txn), splitBySourceID[leg.SourceID].Amount
		leg.Account, leg.Currency, leg.Closed, leg.Active = acct.Name, acct.Currency, acct.Closed, acct.Active
	}
	slices.SortFunc(out, func(a, b store.OneSidedTransfer) int {
		if c := ix.compareTransactions(txnOf(a), txnOf(b)); c != 0 {
			return c
		}
		return cmp.Compare(a.SourceID, b.SourceID)
	})
	return out
}

// checkBalances compares reconciled sums against statements; an account
// missing from statements is never reconciled, not mismatched.
func checkBalances(rows store.Rows, statements map[string]parsedStatement) store.BalanceCheck {
	reconciledSums := make(map[string]int64, len(rows.Accounts))
	for _, txn := range rows.Transactions {
		if txn.Status == "reconciled" {
			reconciledSums[txn.AccountID] += txn.Amount
		}
	}

	var check store.BalanceCheck
	for _, acct := range rows.Accounts {
		if store.IsInvestmentAccount(acct.Type) {
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

// byNameThenSourceID orders by name (byte order) then numeric source id,
// the tie-break a string comparison of "9" and "10" would invert.
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

// checkSplits verifies every transaction's splits sum to its amount,
// sorting Mismatched by compareTransactions.
func checkSplits(ix rowIndex, rows store.Rows) store.SplitCheck {
	sums := make(map[string]int64, len(rows.Transactions))
	counts := make(map[string]int, len(rows.Transactions))
	for _, s := range rows.Splits {
		sums[s.TransactionID] += s.Amount
		counts[s.TransactionID]++
	}

	check := store.SplitCheck{Checked: len(rows.Transactions)}
	var mismatched []store.Transaction
	for _, txn := range rows.Transactions {
		if counts[txn.ID] > 0 && sums[txn.ID] == txn.Amount {
			continue
		}
		mismatched = append(mismatched, txn)
	}
	slices.SortFunc(mismatched, ix.compareTransactions)

	for _, txn := range mismatched {
		acct := ix.accounts[txn.AccountID]
		check.Mismatched = append(check.Mismatched, store.SplitMismatch{
			ID: txn.ID, SourceID: txn.SourceID, Date: txn.Date, Account: acct.Name, Currency: txn.Currency,
			Closed: acct.Closed, Active: acct.Active, Payee: ix.payee(txn), Amount: txn.Amount, SplitsTotal: sums[txn.ID],
		})
	}
	return check
}

// describeShareMismatches fills each holding's display fields and Difference, then
// sorts by account name, account source id, security name, security source id.
func describeShareMismatches(rows store.Rows, mismatched []store.ShareMismatch) []store.ShareMismatch {
	accounts := make(map[string]store.Account, len(rows.Accounts))
	for _, a := range rows.Accounts {
		accounts[a.ID] = a
	}
	securities := make(map[string]store.Security, len(rows.Securities))
	for _, s := range rows.Securities {
		securities[s.ID] = s
	}

	out := slices.Clone(mismatched)
	for i := range out {
		m := &out[i]
		acct, sec := accounts[m.AccountID], securities[m.SecurityID]
		m.Account, m.Currency, m.Closed, m.Active, m.AccountSourceID = acct.Name, acct.Currency, acct.Closed, acct.Active, acct.SourceID
		m.Security, m.Ticker, m.SecuritySourceID = sec.Name, sec.Ticker, sec.SourceID
		m.Difference = saturatingSub(m.Quarry, m.Quicken)
	}
	slices.SortFunc(out, func(a, b store.ShareMismatch) int {
		return cmp.Or(
			strings.Compare(a.Account, b.Account),
			cmp.Compare(a.AccountSourceID, b.AccountSourceID),
			strings.Compare(a.Security, b.Security),
			cmp.Compare(a.SecuritySourceID, b.SecuritySourceID),
		)
	})
	return out
}

// saturatingSub returns a - b, clamped to the int64 range instead of wrapping.
func saturatingSub(a, b int64) int64 {
	switch {
	case b > 0 && a < math.MinInt64+b:
		return math.MinInt64
	case b < 0 && a > math.MaxInt64+b:
		return math.MaxInt64
	}
	return a - b
}
