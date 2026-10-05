package report

import (
	"context"

	"github.com/koblas/quarry/internal/store"
)

// ValueReads is the part of Store that values what accounts hold and what it cost: holdings, net worth and
// the investment history an adjusted cost base is walked from.
type ValueReads interface {
	// Holdings lists what each account holds of each security on params.AsOf, with its value.
	Holdings(ctx context.Context, params store.HoldingsParams) (store.Holdings, error)
	// NetWorth lists the balance of each account type and currency on each of params.Dates.
	NetWorth(ctx context.Context, params store.NetWorthParams) (store.NetWorth, error)
	// InvestmentHistory lists every account, security and investment transaction with a security, and every
	// exchange rate.
	InvestmentHistory(ctx context.Context) (store.InvestmentHistory, error)
}

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
	// Charges lists every charge dated through params.Through, with the span of the store's
	// transactions (of the named reported accounts' when params.AccountIDs is set).
	Charges(ctx context.Context, params store.ChargeParams) (store.Charges, error)
	// Search lists the newest params.Limit transactions matching params, with the full match count and the
	// span of the store's transactions (of the named accounts' when params.AccountIDs is set).
	Search(ctx context.Context, params store.SearchParams) (store.Search, error)
	ValueReads
	// Findings lists every finding in the store with its items, and every account.
	Findings(ctx context.Context) (store.FindingList, error)
	// Schema describes what the store holds: its tables and views, accounts, categories and transaction dates.
	Schema(ctx context.Context) (store.Schema, error)
	// Query runs query verbatim and returns at most maxRows rows, every row when maxRows is 0.
	Query(ctx context.Context, query string, maxRows int) (store.QueryResult, error)
}
