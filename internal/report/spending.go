package report

import (
	"context"

	"github.com/koblas/quarry/internal/platform/money"
	"github.com/koblas/quarry/internal/store"
)

// SpendRequest is what a spend read needs from its caller: the window to
// count, resolved by ParseWindow or DefaultWindow, what to group by
// (category when unset), and the accounts to count (each an id or a name;
// none means every account).
type SpendRequest struct {
	Window   store.Window
	By       store.SpendingGroup
	Accounts []string
	// Currency is the currency to report in; the zero value, money.Native, converts nothing.
	Currency money.Currency
}

// SpendingRow is one row of a spend read: what the store found, or for a
// filled month zero spending. Partial marks a month the window cuts short.
type SpendingRow struct {
	store.SpendingRow

	Partial bool
}

// Spending is a spend read: the window it covered, what it was grouped by,
// the accounts it counted and what the store found.
type Spending struct {
	Rows   []SpendingRow
	Totals []store.SpendingTotal
	// MultiTagSplits counts the splits carrying more than one tag; it is set
	// only when grouping by tag.
	MultiTagSplits int
	// Transactions is store.Spending.Transactions.
	Transactions store.TransactionRange
	// Unconverted is store.Spending.Unconverted.
	Unconverted store.Unconverted

	Window store.Window
	By     store.SpendingGroup
	// Accounts is the accounts the request named, in the order given and
	// without repeats; empty means every account was counted.
	Accounts []store.Account
	// Currency is the currency the request asked to report in; individual rows and
	// totals still carry their own, because a split with no rate stays native.
	Currency money.Currency
}

// Empty is whether the window held no spending at all: a currency whose spending nets to zero
// still has a total, so it is not empty.
func (s Spending) Empty() bool { return len(s.Totals) == 0 }

// spendCommand names spend in its refusals; it equals the cli command word.
const spendCommand = "spend"

// Spend reads the spending inside req.Window, grouped by req.By, in the accounts req.Accounts
// name (every account when none). An account it cannot pick is a RefusalError.
func (s *Server) Spend(ctx context.Context, req SpendRequest) (Spending, error) {
	accounts, accountIDs, err := s.namedAccounts(ctx, spendCommand, req.Accounts)
	if err != nil {
		return Spending{}, err
	}
	spending, err := s.store.Spending(ctx, store.SpendingParams{Window: req.Window, By: req.By, AccountIDs: accountIDs, Currency: req.Currency})
	if err != nil {
		return Spending{}, s.readRefusal(ctx, spendCommand, err)
	}
	result := Spending{
		Totals:         spending.Totals,
		MultiTagSplits: spending.MultiTagSplits,
		Transactions:   spending.Transactions,
		Unconverted:    spending.Unconverted,
		Window:         req.Window,
		By:             req.By,
		Accounts:       accounts,
		Currency:       req.Currency,
	}
	if req.By == store.SpendByMonth {
		result.Rows = fillSeries(monthSeries(req.Window), currencyList(spending.Totals, spendingTotalCurrency), fillTarget(req.Currency), spending.Rows, spendingRowPeriod, blankSpendingRow, wrapSpendingRow)
		return result, nil
	}
	for _, r := range spending.Rows {
		result.Rows = append(result.Rows, wrapSpendingRow(r, false))
	}
	return result, nil
}

// spendingTotalCurrency is the currency of a spending total.
func spendingTotalCurrency(t store.SpendingTotal) string { return t.Currency }

// spendingRowPeriod is the month and currency a month-grouped row reports.
func spendingRowPeriod(r store.SpendingRow) periodKey {
	return periodKey{label: *r.Key, currency: r.Currency}
}

// blankSpendingRow is the row of a month the store found no spending in.
func blankSpendingRow(k periodKey) store.SpendingRow {
	return store.SpendingRow{Key: &k.label, Currency: k.currency}
}

// wrapSpendingRow is a spend row with the month's Partial.
func wrapSpendingRow(r store.SpendingRow, partial bool) SpendingRow {
	return SpendingRow{SpendingRow: r, Partial: partial}
}
