package importer

import "github.com/koblas/quarry/internal/store"

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
				ID: acct.ID, Name: acct.Name, Currency: acct.Currency, Closed: acct.Closed, Active: acct.Active,
				StatementDate: stmt.Date, Quarry: sum, Quicken: stmt.Cents, Difference: sum - stmt.Cents,
			})
		}
	}
	return check
}

// checkSplits verifies every imported transaction has at least one split
// and its splits sum to its amount.
func checkSplits(rows store.Rows) store.SplitCheck {
	sums := make(map[string]int64, len(rows.Transactions))
	counts := make(map[string]int, len(rows.Transactions))
	for _, s := range rows.Splits {
		sums[s.TransactionID] += s.Amount
		counts[s.TransactionID]++
	}
	accountNames := make(map[string]string, len(rows.Accounts))
	for _, a := range rows.Accounts {
		accountNames[a.ID] = a.Name
	}
	payeeNames := make(map[string]string, len(rows.Payees))
	for _, p := range rows.Payees {
		payeeNames[p.ID] = p.Name
	}

	check := store.SplitCheck{Checked: len(rows.Transactions)}
	for _, txn := range rows.Transactions {
		if counts[txn.ID] > 0 && sums[txn.ID] == txn.Amount {
			continue
		}
		var payee string
		if txn.PayeeID != nil {
			payee = payeeNames[*txn.PayeeID]
		}
		check.Mismatched = append(check.Mismatched, store.SplitMismatch{
			ID: txn.ID, Date: txn.Date, Account: accountNames[txn.AccountID], Currency: txn.Currency,
			Payee: payee, Amount: txn.Amount, SplitsTotal: sums[txn.ID],
		})
	}
	return check
}
