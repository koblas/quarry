package duckstore

import (
	"context"
	"database/sql"
	"time"

	"github.com/koblas/quarry/internal/store"
)

// searchOrder is newest first, ties by descending source id then id, so a transaction's rows never
// interleave with another's; the cut and the final listing both use it.
const searchOrder = "txn_date DESC, txn_source_id DESC, txn_id DESC"

// Search parameters, numbered in the order searchArgs binds them; the named accounts follow from searchFirstAccount.
const (
	searchSince        = "$1"
	searchUntil        = "$2"
	searchLimit        = "$3"
	searchText         = "$4"
	searchMin          = "$5"
	searchMax          = "$6"
	searchCategory     = "$7"
	searchFirstAccount = 8
)

// Cents of an amount, an unsigned amount and a split's; DECIMAL(38,2) keeps the product from overflowing.
const (
	searchTxnCents    = `CAST(CAST(t.amount AS DECIMAL(38,2)) * 100 AS BIGINT)`
	searchAbsTxnCents = `CAST(CAST(abs(t.amount) AS DECIMAL(38,2)) * 100 AS BIGINT)`
	searchSplitCents  = `CAST(CAST(s.amount AS DECIMAL(38,2)) * 100 AS BIGINT)`
)

// searchTextMatch keeps a transaction whose payee, memo or a split memo contains the text, ignoring case; no text keeps all.
const searchTextMatch = `(CAST(` + searchText + ` AS VARCHAR) IS NULL
			OR contains(lower(p.name), lower(` + searchText + `))
			OR contains(lower(t.memo), lower(` + searchText + `))
			OR EXISTS (SELECT 1 FROM splits ms WHERE ms.transaction_id = t.id AND contains(lower(ms.memo), lower(` + searchText + `))))`

// searchAmountRange keeps a transaction whose unsigned amount is within the inclusive bounds; no bound keeps all.
const searchAmountRange = `(CAST(` + searchMin + ` AS BIGINT) IS NULL OR ` + searchAbsTxnCents + ` >= ` + searchMin + `)
			AND (CAST(` + searchMax + ` AS BIGINT) IS NULL OR ` + searchAbsTxnCents + ` <= ` + searchMax + `)`

// searchCategoryMatch keeps a transaction with a split in the category or under it, ignoring case; no category keeps all.
const searchCategoryMatch = `(CAST(` + searchCategory + ` AS VARCHAR) IS NULL
				OR EXISTS (SELECT 1 FROM splits cs JOIN categories cc ON cc.id = cs.category_id
					WHERE cs.transaction_id = t.id
					AND (lower(cc.full_path) = lower(` + searchCategory + `)
						OR starts_with(lower(cc.full_path), lower(` + searchCategory + `) || ':'))))`

// searchRowsQuery reads each matching transaction's splits in split source id order. The transaction-grain
// CTE counts every match before the limit cuts, so a transaction's splits are never counted or cut.
func searchRowsQuery(accounts accountFilter) string {
	var accountClause string
	if len(accounts) > 0 {
		accountClause = " AND t.account_id IN (" + accounts.marks(searchFirstAccount) + ")"
	}
	return `
WITH m AS (
	SELECT t.id AS txn_id, t.date AS txn_date, t.source_id AS txn_source_id,
		a.id AS account_id, a.name AS account_name, a.currency AS account_currency, a.closed, a.active,
		p.name AS payee, NULLIF(t.memo, '') AS memo, ` + searchTxnCents + ` AS amount, t.currency,
		EXISTS (SELECT 1 FROM splits ts WHERE ts.transaction_id = t.id AND ` + transferLeg("ts") + `) AS transfer,
		NOT (` + reportedTransaction + `) AS excluded,
		count(*) OVER () AS matched
	FROM transactions t
	JOIN accounts a ON a.id = t.account_id
	LEFT JOIN payees p ON p.id = t.payee_id
	WHERE (` + searchSince + ` IS NULL OR t.date >= CAST(` + searchSince + ` AS DATE))
		AND (` + searchUntil + ` IS NULL OR t.date <= CAST(` + searchUntil + ` AS DATE))
		AND ` + searchTextMatch + `
			AND ` + searchAmountRange + `
			AND ` + searchCategoryMatch + accountClause + `
	ORDER BY ` + searchOrder + `
	LIMIT ` + searchLimit + `
)
SELECT m.txn_id, m.txn_date, m.account_id, m.account_name, m.account_currency, m.closed, m.active,
	m.payee, m.memo, m.amount, m.currency, m.transfer, m.excluded, m.matched,
	c.full_path, NULLIF(s.memo, ''), ` + searchSplitCents + `, ` + transferLeg("s") + `
FROM m
LEFT JOIN splits s ON s.transaction_id = m.txn_id
LEFT JOIN categories c ON c.id = s.category_id
ORDER BY ` + searchOrder + `, s.source_id, s.id`
}

// searchSpanQuery is the first and last day of every transaction, or of the named accounts', and whether $1,
// the category, names none; its other parameters are those accounts, from $2.
func searchSpanQuery(accounts accountFilter) string {
	const unknownCategory = "NOT (CAST($1 AS VARCHAR) IS NULL OR EXISTS (SELECT 1 FROM categories WHERE lower(full_path) = lower($1)))"
	if len(accounts) == 0 {
		return "SELECT min(date), max(date), " + unknownCategory + " FROM transactions"
	}
	return "SELECT min(date), max(date), " + unknownCategory + " FROM transactions WHERE account_id IN (" + accounts.marks(2) + ")"
}

// Search lists the newest params.Limit transactions matching params, as store.Search documents.
// A store it cannot open or read is a *store.OpenError.
func (s *Store) Search(ctx context.Context, params store.SearchParams) (store.Search, error) {
	db, err := s.openRead(ctx)
	if err != nil {
		return store.Search{}, err
	}
	defer func() { _ = db.Close() }()

	accounts := accountFilter(params.AccountIDs)
	var found store.Search
	err = db.QueryRows(ctx, searchRowsQuery(accounts), searchArgs(params), func(scan func(dest ...any) error) error {
		return scanSearchRow(scan, &found)
	})
	if err == nil {
		spanArgs := append([]any{searchCategoryArg(params.Category)}, accounts.args()...)
		err = db.QueryRows(ctx, searchSpanQuery(accounts), spanArgs, func(scan func(dest ...any) error) error {
			var first, last sql.NullTime
			if err := scan(&first, &last, &found.UnknownCategory); err != nil {
				return err
			}
			found.Transactions = store.TransactionRange{First: first.Time, Last: last.Time}
			return nil
		})
	}
	if err != nil {
		return store.Search{}, openFault(s.Path(), err)
	}
	return found, nil
}

// searchArgs binds the rows statement: the window's open bounds, an unlimited Limit of 0, no text, no category and open amount bounds as NULL, then the accounts.
func searchArgs(params store.SearchParams) []any {
	var limit any
	if params.Limit > 0 {
		limit = int64(params.Limit)
	}
	accounts := accountFilter(params.AccountIDs).args()
	var text any
	if params.Text != "" {
		text = params.Text
	}
	args := make([]any, 0, searchFirstAccount-1+len(accounts))
	args = append(args, searchDay(params.Window.Since), searchDay(params.Window.Until), limit, text,
		searchCents(params.Min), searchCents(params.Max), searchCategoryArg(params.Category))
	return append(args, accounts...)
}

// searchCategoryArg is bound as the category path, or NULL for none.
func searchCategoryArg(category *string) any {
	if category == nil {
		return nil
	}
	return *category
}

// searchCents is bound as the amount in cents, or NULL for an open bound.
func searchCents(bound *int64) any {
	if bound == nil {
		return nil
	}
	return *bound
}

// searchDay is bound as the day's civil date, or NULL for an open bound.
func searchDay(bound *time.Time) any {
	if bound == nil {
		return nil
	}
	return civilDay(*bound)
}

// scanSearchRow reads one searchRowsQuery row through scan into found: a new SearchRow when the transaction
// differs from the last one, then its split, if the transaction has any.
func scanSearchRow(scan func(dest ...any) error, found *store.Search) error {
	var row store.SearchRow
	var payee, memo, category, splitMemo sql.NullString
	var splitAmount sql.NullInt64
	var splitTransfer sql.NullBool
	err := scan(&row.TransactionID, &row.Date, &row.Account.ID, &row.Account.Name, &row.Account.Currency,
		&row.Account.Closed, &row.Account.Active, &payee, &memo, &row.Amount, &row.Currency,
		&row.Transfer, &row.Excluded, &found.Matched, &category, &splitMemo, &splitAmount, &splitTransfer)
	if err != nil {
		return err
	}
	if n := len(found.Rows); n == 0 || found.Rows[n-1].TransactionID != row.TransactionID {
		row.Payee, row.Memo = nullStringPtr(payee), nullStringPtr(memo)
		found.Rows = append(found.Rows, row)
	}
	if splitAmount.Valid {
		last := &found.Rows[len(found.Rows)-1]
		last.Splits = append(last.Splits, store.SearchSplit{
			Category: nullStringPtr(category), Memo: nullStringPtr(splitMemo), Amount: splitAmount.Int64, Transfer: splitTransfer.Bool,
		})
	}
	return nil
}
