package report

import (
	"context"

	"github.com/koblas/quarry/internal/store"
)

// CashFlowRequest is what a cash-flow read needs from its caller: the window to count,
// the period unit, and the accounts to count (each an id or a name; none means every account).
type CashFlowRequest struct {
	Window   store.Window
	By       store.CashFlowPeriod
	Accounts []string
}

// CashFlowRow is one period's cash flow in one currency: what the store found, or for a
// filled period zero income and spending with no savings rate. Partial marks a period the
// window cuts short.
type CashFlowRow struct {
	store.CashFlowRow

	Partial bool
}

// CashFlow is a cash-flow read: the window it covered, the period unit, the accounts it
// counted and what the store found.
type CashFlow struct {
	Rows   []CashFlowRow
	Totals []store.CashFlowTotal
	// Transactions is store.CashFlow.Transactions.
	Transactions store.TransactionRange

	Window store.Window
	By     store.CashFlowPeriod
	// Accounts is the accounts the request named, in the order given and without repeats;
	// empty means every account was counted.
	Accounts []store.Account
}

// Empty is whether the window held no income or spending at all: a currency that nets to
// zero still has a total, so it is not empty.
func (c CashFlow) Empty() bool { return len(c.Totals) == 0 }

// cashFlowCommand names cashflow in its refusals; it equals the cli command word.
const cashFlowCommand = "cashflow"

// CashFlow reads the income and spending inside req.Window, per req.By period, in the
// accounts req.Accounts name (every account when none). An account it cannot pick is a RefusalError.
func (s *Server) CashFlow(ctx context.Context, req CashFlowRequest) (CashFlow, error) {
	accounts, accountIDs, err := s.namedAccounts(ctx, cashFlowCommand, req.Accounts)
	if err != nil {
		return CashFlow{}, err
	}
	flow, err := s.store.CashFlow(ctx, store.CashFlowParams{Window: req.Window, By: req.By, AccountIDs: accountIDs})
	if err != nil {
		return CashFlow{}, s.readRefusal(ctx, cashFlowCommand, err)
	}
	return CashFlow{
		Rows:         fillSeries(cashFlowSeries(req), currencyList(flow.Totals, cashFlowTotalCurrency), flow.Rows, cashFlowRowPeriod, blankCashFlowRow, wrapCashFlowRow),
		Totals:       flow.Totals,
		Transactions: flow.Transactions,
		Window:       req.Window,
		By:           req.By,
		Accounts:     accounts,
	}, nil
}

// cashFlowTotalCurrency is the currency of a cash-flow total.
func cashFlowTotalCurrency(t store.CashFlowTotal) string { return t.Currency }

// cashFlowRowPeriod is the period and currency a row reports.
func cashFlowRowPeriod(r store.CashFlowRow) periodKey {
	return periodKey{label: r.Period, currency: r.Currency}
}

// blankCashFlowRow is the row of a period the store found no income or spending in.
func blankCashFlowRow(k periodKey) store.CashFlowRow {
	return store.CashFlowRow{Period: k.label, Currency: k.currency}
}

// wrapCashFlowRow is a cash-flow row with the period's Partial.
func wrapCashFlowRow(r store.CashFlowRow, partial bool) CashFlowRow {
	return CashFlowRow{CashFlowRow: r, Partial: partial}
}

// cashFlowSeries is the periods of the window in the request's unit.
func cashFlowSeries(req CashFlowRequest) []period {
	if req.By == store.CashFlowByYear {
		return yearSeries(req.Window)
	}
	return monthSeries(req.Window)
}
