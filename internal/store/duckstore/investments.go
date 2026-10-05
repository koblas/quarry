package duckstore

import (
	"context"
	"database/sql"

	"github.com/koblas/quarry/internal/store"
)

// investmentSecuritiesQuery reads every security the investment transactions can name.
const investmentSecuritiesQuery = `SELECT id, source_id, name, ticker, currency FROM securities ORDER BY id`

// investmentTransactionsQuery reads every investment transaction with a security, as exact integers: shares and
// split sides in millionths, amount and cost basis in cents, commission in ten-thousandths.
const investmentTransactionsQuery = `SELECT id, source_id, account_id, security_id, date, action,
	CAST(shares * 1000000 AS BIGINT), CAST(amount * 100 AS BIGINT), CAST(commission * 10000 AS BIGINT),
	CAST(cost_basis * 100 AS BIGINT), currency, memo,
	CAST(split_new_shares * 1000000 AS BIGINT), CAST(split_old_shares * 1000000 AS BIGINT)
FROM investment_transactions
WHERE security_id IS NOT NULL
ORDER BY date, source_id`

// InvestmentHistory reads every account, security, investment transaction with a security, and exchange rate in one
// open. It refuses a store it cannot open or read with *store.OpenError.
func (s *Store) InvestmentHistory(ctx context.Context) (store.InvestmentHistory, error) {
	db, err := s.openRead(ctx)
	if err != nil {
		return store.InvestmentHistory{}, err
	}
	defer func() { _ = db.Close() }()

	var history store.InvestmentHistory
	if history.Accounts, err = readAccounts(ctx, db); err != nil {
		return store.InvestmentHistory{}, openFault(s.Path(), err)
	}
	if history.Securities, err = readSecurities(ctx, db); err != nil {
		return store.InvestmentHistory{}, openFault(s.Path(), err)
	}
	if history.Transactions, err = readInvestmentTransactions(ctx, db); err != nil {
		return store.InvestmentHistory{}, openFault(s.Path(), err)
	}
	if history.Rates, _, err = readRates(ctx, db); err != nil {
		return store.InvestmentHistory{}, openFault(s.Path(), err)
	}
	return history, nil
}

// readSecurities reads every security, sorted by id.
func readSecurities(ctx context.Context, db ReadDB) ([]store.Security, error) {
	var securities []store.Security
	err := db.QueryRows(ctx, investmentSecuritiesQuery, nil, func(scan func(dest ...any) error) error {
		var sec store.Security
		var ticker, currency sql.NullString
		if err := scan(&sec.ID, &sec.SourceID, &sec.Name, &ticker, &currency); err != nil {
			return err
		}
		sec.Ticker, sec.Currency = nullStringPtr(ticker), nullStringPtr(currency)
		securities = append(securities, sec)
		return nil
	})
	return securities, err //nolint:wrapcheck // the caller classifies the driver's own error with openFault
}

// readInvestmentTransactions reads every investment transaction with a security in date then source id order.
func readInvestmentTransactions(ctx context.Context, db ReadDB) ([]store.InvestmentTransaction, error) {
	var txns []store.InvestmentTransaction
	err := db.QueryRows(ctx, investmentTransactionsQuery, nil, func(scan func(dest ...any) error) error {
		var t store.InvestmentTransaction
		var securityID string
		var memo sql.NullString
		var shares, commission, costBasis, splitNew, splitOld sql.NullInt64
		err := scan(&t.ID, &t.SourceID, &t.AccountID, &securityID, &t.Date, &t.Action,
			&shares, &t.Amount, &commission, &costBasis, &t.Currency, &memo, &splitNew, &splitOld)
		if err != nil {
			return err
		}
		t.SecurityID = &securityID
		t.Shares, t.Commission, t.CostBasis = nullInt64Ptr(shares), nullInt64Ptr(commission), nullInt64Ptr(costBasis)
		t.Memo = nullStringPtr(memo)
		t.SplitNewShares, t.SplitOldShares = nullInt64Ptr(splitNew), nullInt64Ptr(splitOld)
		txns = append(txns, t)
		return nil
	})
	return txns, err //nolint:wrapcheck // the caller classifies the driver's own error with openFault
}
