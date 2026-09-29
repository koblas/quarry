package report

import (
	"context"

	"github.com/koblas/quarry/internal/store"
)

// Store is the read side of quarry's store: each method opens the store
// read-only, answers, and closes it again.
type Store interface {
	// Status describes the store and the import run that built it.
	Status(ctx context.Context) (store.Status, error)
	// Accounts lists every account with its balance.
	Accounts(ctx context.Context) (store.AccountList, error)
	// Spending reads spending in params.Window, grouped and filtered as params says.
	Spending(ctx context.Context, params store.SpendingParams) (store.Spending, error)
	// CashFlow reads income and spending in params.Window, per period and currency, filtered as params says.
	CashFlow(ctx context.Context, params store.CashFlowParams) (store.CashFlow, error)
	// Query runs query verbatim and returns at most maxRows rows, every row when maxRows is 0.
	Query(ctx context.Context, query string, maxRows int) (store.QueryResult, error)
}
