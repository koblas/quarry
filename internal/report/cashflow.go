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

// CashFlow reads the income and spending inside req.Window, per req.By period, in the
// accounts req.Accounts name (every account when none). An account it cannot pick is a RefusalError.
func (s *Server) CashFlow(ctx context.Context, req CashFlowRequest) (CashFlow, error) {
	accounts, accountIDs, err := s.namedAccounts(ctx, "cashflow", req.Accounts)
	if err != nil {
		return CashFlow{}, err
	}
	flow, err := s.store.CashFlow(ctx, store.CashFlowParams{Window: req.Window, By: req.By, AccountIDs: accountIDs})
	if err != nil {
		return CashFlow{}, s.readRefusal(ctx, "cashflow", err)
	}
	return CashFlow{
		Rows:         fillPeriods(flow, req),
		Totals:       flow.Totals,
		Transactions: flow.Transactions,
		Window:       req.Window,
		By:           req.By,
		Accounts:     accounts,
	}, nil
}

// fillPeriods is a row for every period of the window in every currency of flow's Totals:
// the store's row where it has one, else zero income and spending with no savings rate.
func fillPeriods(flow store.CashFlow, req CashFlowRequest) []CashFlowRow {
	type periodCurrency struct{ label, currency string }
	found := make(map[periodCurrency]store.CashFlowRow, len(flow.Rows))
	for _, r := range flow.Rows {
		found[periodCurrency{r.Period, r.Currency}] = r
	}
	series := monthSeries(req.Window)
	if req.By == store.CashFlowByYear {
		series = yearSeries(req.Window)
	}
	rows := make([]CashFlowRow, 0, len(series)*len(flow.Totals))
	for _, p := range series {
		for _, total := range flow.Totals {
			row, ok := found[periodCurrency{p.Label, total.Currency}]
			if !ok {
				row = store.CashFlowRow{Period: p.Label, Currency: total.Currency}
			}
			rows = append(rows, CashFlowRow{CashFlowRow: row, Partial: p.Partial})
		}
	}
	return rows
}
